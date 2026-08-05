package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guerrero/gitia/internal/git"
)

// newRepo creates an initialized git repository in a temp directory with a
// deterministic identity, and returns its path.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return dir
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestRepoRoot(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	write(t, dir, "nested/deep/file.txt", "hi\n")

	root, err := git.RepoRoot(ctx, filepath.Join(dir, "nested", "deep"))
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}

	// macOS temp dirs are symlinked through /private, so compare resolved paths.
	want, _ := filepath.EvalSymlinks(dir)
	got, _ := filepath.EvalSymlinks(root)
	if got != want {
		t.Errorf("RepoRoot() = %q, want %q", got, want)
	}
}

func TestRepoRootOutsideARepository(t *testing.T) {
	if _, err := git.RepoRoot(context.Background(), t.TempDir()); err == nil {
		t.Fatal("RepoRoot() = nil error outside a repository, want an error")
	}
}

func TestHasStagedChanges(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)

	staged, err := git.HasStagedChanges(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if staged {
		t.Error("HasStagedChanges() = true on a fresh repository, want false")
	}

	write(t, dir, "a.txt", "hello\n")
	run(t, dir, "add", "a.txt")

	if staged, err = git.HasStagedChanges(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if !staged {
		t.Error("HasStagedChanges() = false after git add, want true")
	}
}

func TestHasStagedChangesIgnoresUnstagedWork(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	write(t, dir, "a.txt", "hello\n")
	run(t, dir, "add", "a.txt")
	run(t, dir, "commit", "-m", "chore: seed")

	write(t, dir, "a.txt", "changed\n") // modified but not staged

	staged, err := git.HasStagedChanges(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if staged {
		t.Error("HasStagedChanges() = true for unstaged work; gitia must only see the index")
	}
}

func TestStagedNameStatus(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	write(t, dir, "keep.txt", "keep\n")
	write(t, dir, "gone.txt", "gone\n")
	write(t, dir, "old.txt", "content that stays identical so git detects a rename\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "chore: seed")

	write(t, dir, "keep.txt", "keep, modified\n")
	write(t, dir, "added.txt", "new\n")
	run(t, dir, "rm", "-q", "gone.txt")
	run(t, dir, "mv", "old.txt", "renamed.txt")
	run(t, dir, "add", ".")

	changes, err := git.StagedNameStatus(ctx, dir)
	if err != nil {
		t.Fatalf("StagedNameStatus: %v", err)
	}

	byPath := map[string]git.FileChange{}
	for _, c := range changes {
		byPath[c.Path] = c
	}

	if c := byPath["added.txt"]; c.Status != "A" {
		t.Errorf("added.txt status = %q, want %q", c.Status, "A")
	}
	if c := byPath["keep.txt"]; c.Status != "M" {
		t.Errorf("keep.txt status = %q, want %q", c.Status, "M")
	}
	if c := byPath["gone.txt"]; c.Status != "D" {
		t.Errorf("gone.txt status = %q, want %q", c.Status, "D")
	}
	if c := byPath["renamed.txt"]; c.Status != "R" || c.OldPath != "old.txt" {
		t.Errorf("renamed.txt = %+v, want Status R and OldPath old.txt", c)
	}
}

func TestStagedNameStatusHandlesPathsWithSpaces(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	write(t, dir, "a file with spaces.txt", "hi\n")
	run(t, dir, "add", ".")

	changes, err := git.StagedNameStatus(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Path != "a file with spaces.txt" {
		t.Errorf("StagedNameStatus() = %+v, want one entry for %q", changes, "a file with spaces.txt")
	}
}

func TestStagedStat(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	write(t, dir, "a.txt", "one\ntwo\n")
	run(t, dir, "add", ".")

	stat, err := git.StagedStat(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stat, "a.txt") {
		t.Errorf("StagedStat() = %q, want it to mention a.txt", stat)
	}
}

func TestStagedFileDiffsSplitsPerPath(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	write(t, dir, "a.txt", "alpha\n")
	write(t, dir, "sub/b.txt", "beta\n")
	run(t, dir, "add", ".")

	diffs, err := git.StagedFileDiffs(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 2 {
		t.Fatalf("StagedFileDiffs() returned %d entries, want 2: %+v", len(diffs), diffs)
	}

	byPath := map[string]string{}
	for _, d := range diffs {
		byPath[d.Path] = d.Patch
	}
	if !strings.Contains(byPath["a.txt"], "+alpha") {
		t.Errorf("a.txt patch = %q, want it to contain +alpha", byPath["a.txt"])
	}
	if strings.Contains(byPath["a.txt"], "beta") {
		t.Error("a.txt patch leaked content from sub/b.txt")
	}
	if !strings.HasPrefix(byPath["sub/b.txt"], "diff --git") {
		t.Errorf("sub/b.txt patch = %q, want it to start with a diff --git header", byPath["sub/b.txt"])
	}
}

func TestCommit(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	write(t, dir, "a.txt", "hi\n")
	run(t, dir, "add", ".")

	msg := "feat(cli): add a thing\n\nA body paragraph.\n"
	if err := git.Commit(ctx, dir, msg, false); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	cmd := exec.Command("git", "log", "-1", "--pretty=%B")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != strings.TrimSpace(msg) {
		t.Errorf("commit message = %q, want %q", got, strings.TrimSpace(msg))
	}
}

func TestCommitSignOff(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	write(t, dir, "a.txt", "hi\n")
	run(t, dir, "add", ".")

	if err := git.Commit(ctx, dir, "chore: sign off\n", true); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "log", "-1", "--pretty=%B")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if !strings.Contains(string(out), "Signed-off-by: Test <test@example.com>") {
		t.Errorf("commit body = %q, want a Signed-off-by trailer", out)
	}
}
