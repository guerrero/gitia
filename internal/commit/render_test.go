package commit_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/guerrero/gitia/internal/commit"
	"github.com/guerrero/gitia/internal/rules"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestRender(t *testing.T) {
	rs := rules.Conventional()

	tests := []struct {
		name   string
		msg    commit.Message
		golden string
	}{
		{
			name:   "subject only",
			msg:    commit.Message{Type: "docs", Subject: "clarify install steps"},
			golden: "subject_only.golden",
		},
		{
			name: "scope and body",
			msg: commit.Message{
				Type:    "feat",
				Scope:   "commit",
				Subject: "generate messages from the staged diff",
				Body: []string{
					"The staged diff is collected with git diff --staged and handed to a local model under Ollama.",
					"Nothing leaves the machine.",
				},
			},
			golden: "scope_and_body.golden",
		},
		{
			name: "breaking change",
			msg: commit.Message{
				Type:                "feat",
				Scope:               "config",
				Subject:             "move config to XDG directory",
				Body:                []string{"The config file now resolves through $XDG_CONFIG_HOME."},
				Breaking:            true,
				BreakingDescription: "config.toml is no longer read from the repository root",
			},
			golden: "breaking.golden",
		},
		{
			name: "footers",
			msg: commit.Message{
				Type:    "fix",
				Scope:   "git",
				Subject: "exit 4 when the index is empty",
				Body:    []string{"An empty staging area is a user error, not a failure."},
				Footers: []commit.Footer{{Token: "Fixes", Value: "#123, #456"}},
			},
			golden: "footers.golden",
		},
		{
			name: "long body wraps at the rule width",
			msg: commit.Message{
				Type:    "refactor",
				Subject: "extract the precedence engine",
				Body: []string{
					"The precedence engine used to live inline in the commit command, which meant the precedence matrix could only be tested through the whole pipeline including git and Ollama. Moving it into internal/rules makes it a pure table test.",
				},
			},
			golden: "wrapped_body.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commit.Render(tt.msg, rs)
			path := filepath.Join("testdata", tt.golden)

			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden: %v (run: go test ./internal/commit/ -update)", err)
			}
			if got != string(want) {
				t.Errorf("Render mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

func TestRenderOmitsBodyWhenDisabled(t *testing.T) {
	rs := rules.Conventional()
	rs.IncludeBody = false

	got := commit.Render(commit.Message{
		Type:    "chore",
		Subject: "bump deps",
		Body:    []string{"this must not appear"},
	}, rs)

	if got != "chore: bump deps\n" {
		t.Errorf("Render() = %q, want %q", got, "chore: bump deps\n")
	}
}

func TestRenderHasExactlyOneTrailingNewline(t *testing.T) {
	rs := rules.Conventional()
	got := commit.Render(commit.Message{
		Type:    "feat",
		Subject: "a",
		Body:    []string{"b"},
		Footers: []commit.Footer{{Token: "Fixes", Value: "#1"}},
	}, rs)

	if len(got) < 2 || got[len(got)-1] != '\n' || got[len(got)-2] == '\n' {
		t.Errorf("Render() = %q; want exactly one trailing newline", got)
	}
}

func TestHeader(t *testing.T) {
	tests := []struct {
		name string
		msg  commit.Message
		want string
	}{
		{"plain", commit.Message{Type: "fix", Subject: "a bug"}, "fix: a bug"},
		{"scoped", commit.Message{Type: "fix", Scope: "cli", Subject: "a bug"}, "fix(cli): a bug"},
		{"breaking", commit.Message{Type: "feat", Breaking: true, Subject: "new api"}, "feat!: new api"},
		{"scoped breaking", commit.Message{Type: "feat", Scope: "api", Breaking: true, Subject: "new api"}, "feat(api)!: new api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commit.Header(tt.msg); got != tt.want {
				t.Errorf("Header() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWrap(t *testing.T) {
	got := commit.Wrap("aaa bbb ccc ddd", 7)
	want := []string{"aaa bbb", "ccc ddd"}

	if len(got) != len(want) {
		t.Fatalf("Wrap() = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Wrap()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWrapKeepsOverlongWordsIntact(t *testing.T) {
	got := commit.Wrap("https://example.com/a/very/long/url/that/exceeds/the/width x", 10)
	if got[0] != "https://example.com/a/very/long/url/that/exceeds/the/width" {
		t.Errorf("Wrap broke an unbreakable token: %q", got[0])
	}
}

func TestWrapZeroWidthIsIdentity(t *testing.T) {
	got := commit.Wrap("a b c", 0)
	if len(got) != 1 || got[0] != "a b c" {
		t.Errorf("Wrap(_, 0) = %q, want single unwrapped element", got)
	}
}
