package prompt_test

import (
	"strings"
	"testing"

	"github.com/guerrero/gitia/internal/git"
	"github.com/guerrero/gitia/internal/prompt"
)

func patch(path string, size int) git.FileDiff {
	return git.FileDiff{
		Path:  path,
		Patch: "diff --git a/" + path + " b/" + path + "\n" + strings.Repeat("+x\n", size/3),
	}
}

func TestMatchesAny(t *testing.T) {
	patterns := []string{"*.lock", "package-lock.json", "go.sum", "*.min.js", "dist/**"}

	tests := []struct {
		path string
		want bool
	}{
		{"bun.lock", true},
		{"package-lock.json", true},
		{"go.sum", true},
		{"app.min.js", true},
		{"dist/bundle.js", true},
		{"dist/nested/deep/bundle.js", true},
		{"src/main.go", false},
		{"src/app.js", false},
		{"notdist/file.js", false},
		{"src/vendor.lock", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := prompt.MatchesAny(tt.path, patterns); got != tt.want {
				t.Errorf("MatchesAny(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestBudgetUnderBudgetKeepsEverything(t *testing.T) {
	in := prompt.DiffInput{
		Stat: " a.go | 2 +-\n b.go | 3 +-\n",
		Changes: []git.FileChange{
			{Status: "M", Path: "a.go"},
			{Status: "M", Path: "b.go"},
		},
		Diffs: []git.FileDiff{patch("a.go", 300), patch("b.go", 300)},
	}

	got := prompt.Budget(in, 32768, nil)

	if got.Degraded {
		t.Error("Degraded = true for a diff well under budget")
	}
	if len(got.Files) != 2 {
		t.Fatalf("Files = %+v, want 2", got.Files)
	}
	for _, f := range got.Files {
		if f.Patch == "" {
			t.Errorf("%s lost its patch while under budget", f.Path)
		}
	}
	if got.Stat != in.Stat {
		t.Error("Stat must always be included verbatim")
	}
}

func TestBudgetDropsExcludedPathsFirst(t *testing.T) {
	in := prompt.DiffInput{
		Stat: "stat",
		Changes: []git.FileChange{
			{Status: "M", Path: "src/main.go"},
			{Status: "M", Path: "package-lock.json"},
		},
		Diffs: []git.FileDiff{
			patch("src/main.go", 600),
			patch("package-lock.json", 5000),
		},
	}

	got := prompt.Budget(in, 2000, []string{"package-lock.json"})

	byPath := map[string]prompt.BudgetedFile{}
	for _, f := range got.Files {
		byPath[f.Path] = f
	}

	if byPath["package-lock.json"].Patch != "" {
		t.Error("package-lock.json kept its patch; excluded paths must drop to stat-only first")
	}
	if byPath["src/main.go"].Patch == "" {
		t.Error("src/main.go lost its patch even though dropping the lockfile was enough")
	}
	if !got.Degraded {
		t.Error("Degraded = false after dropping a patch")
	}
}

func TestBudgetDropsTheLargestRemainingFilesNext(t *testing.T) {
	in := prompt.DiffInput{
		Stat: "stat",
		Changes: []git.FileChange{
			{Status: "M", Path: "small.go"},
			{Status: "M", Path: "medium.go"},
			{Status: "M", Path: "huge.go"},
		},
		Diffs: []git.FileDiff{
			patch("small.go", 300),
			patch("medium.go", 900),
			patch("huge.go", 9000),
		},
	}

	got := prompt.Budget(in, 1500, nil)

	byPath := map[string]prompt.BudgetedFile{}
	for _, f := range got.Files {
		byPath[f.Path] = f
	}

	if byPath["huge.go"].Patch != "" {
		t.Error("huge.go kept its patch; the largest file must be dropped first")
	}
	if byPath["small.go"].Patch == "" {
		t.Error("small.go lost its patch unnecessarily")
	}
}

func TestBudgetAlwaysListsEveryFile(t *testing.T) {
	in := prompt.DiffInput{
		Stat: "stat",
		Changes: []git.FileChange{
			{Status: "M", Path: "a.go"},
			{Status: "A", Path: "b.go"},
			{Status: "D", Path: "c.go"},
			{Status: "R", Path: "new.go", OldPath: "old.go"},
			{Status: "M", Path: "image.png"}, // binary: no patch from git
		},
		Diffs: []git.FileDiff{patch("a.go", 90000)},
	}

	got := prompt.Budget(in, 100, nil)

	if len(got.Files) != 5 {
		t.Fatalf("Files = %+v, want all 5 paths listed even when every patch is dropped", got.Files)
	}

	byPath := map[string]prompt.BudgetedFile{}
	for _, f := range got.Files {
		byPath[f.Path] = f
	}
	if byPath["c.go"].Status != "D" {
		t.Errorf("c.go status = %q, want D", byPath["c.go"].Status)
	}
	if byPath["new.go"].OldPath != "old.go" {
		t.Errorf("new.go OldPath = %q, want old.go", byPath["new.go"].OldPath)
	}
	if byPath["image.png"].Patch != "" {
		t.Error("image.png has a patch; a binary file must come through stat-only")
	}
}

func TestBudgetNotesExplainTheDegradation(t *testing.T) {
	in := prompt.DiffInput{
		Stat:    "stat",
		Changes: []git.FileChange{{Status: "M", Path: "go.sum"}},
		Diffs:   []git.FileDiff{patch("go.sum", 9000)},
	}

	got := prompt.Budget(in, 100, []string{"go.sum"})

	if len(got.Notes) == 0 {
		t.Fatal("Notes is empty; degradation must be explainable under --verbose")
	}
	if !strings.Contains(strings.Join(got.Notes, "\n"), "go.sum") {
		t.Errorf("Notes = %v, want them to name the dropped path", got.Notes)
	}
}

func TestBudgetZeroMaxBytesMeansUnlimited(t *testing.T) {
	in := prompt.DiffInput{
		Stat:    "stat",
		Changes: []git.FileChange{{Status: "M", Path: "a.go"}},
		Diffs:   []git.FileDiff{patch("a.go", 90000)},
	}

	got := prompt.Budget(in, 0, nil)
	if got.Degraded || got.Files[0].Patch == "" {
		t.Error("a max of 0 must disable budgeting entirely")
	}
}
