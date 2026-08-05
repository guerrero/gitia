package rules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guerrero/gitia/internal/rules"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExtractSectionsSelectsMatchingHeadings(t *testing.T) {
	md := `# Project

Some prose that must not be included.

## Build

Run make build.

## Commit conventions

Use imperative mood.

### Scopes

Scope by package.

## Testing

Run go test.

## Versioning policy

Semver.
`

	got := rules.ExtractSections(md, 2000)

	for _, want := range []string{"Commit conventions", "imperative mood", "Scopes", "Scope by package", "Versioning policy", "Semver"} {
		if !strings.Contains(got, want) {
			t.Errorf("ExtractSections() is missing %q\n--- got ---\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"Run make build", "Run go test", "must not be included"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("ExtractSections() wrongly included %q\n--- got ---\n%s", unwanted, got)
		}
	}
}

func TestExtractSectionsMatchesCaseInsensitively(t *testing.T) {
	md := "## GIT WORKFLOW\n\nRebase, do not merge.\n"
	if !strings.Contains(rules.ExtractSections(md, 2000), "Rebase") {
		t.Error("ExtractSections() did not match an uppercase heading")
	}
}

func TestExtractSectionsStopsAtASiblingHeading(t *testing.T) {
	md := "## Commit\n\nkeep me\n\n## Build\n\ndrop me\n"
	got := rules.ExtractSections(md, 2000)

	if !strings.Contains(got, "keep me") {
		t.Error("ExtractSections() dropped the matching section")
	}
	if strings.Contains(got, "drop me") {
		t.Error("ExtractSections() ran past the sibling heading")
	}
}

func TestExtractSectionsFallsBackToTheWholeFileWhenSmall(t *testing.T) {
	md := "Just prose about how we write commits, with no headings at all.\n"
	if got := rules.ExtractSections(md, 2000); got != md {
		t.Errorf("ExtractSections() = %q, want the whole file as a fallback", got)
	}
}

func TestExtractSectionsReturnsNothingForALargeUnmatchedFile(t *testing.T) {
	md := "# Build\n\n" + strings.Repeat("filler filler filler\n", 300)
	if got := rules.ExtractSections(md, 2000); got != "" {
		t.Errorf("ExtractSections() = %q, want empty for a large file with no matching heading", got)
	}
}

func TestExtractSectionsIgnoresHeadingsInsideCodeFences(t *testing.T) {
	md := "## Build\n\n```sh\n# Commit the result\nmake build\n```\n\n## Style\n\ntabs\n"
	if got := rules.ExtractSections(md, 2000); strings.Contains(got, "make build") {
		t.Errorf("ExtractSections() treated a comment inside a code fence as a heading\n%s", got)
	}
}

func TestDiscoverAgentDocsOrdersLeastAuthoritativeFirst(t *testing.T) {
	root := t.TempDir()
	cfg := rules.DefaultConfig().Agents

	writeFile(t, root, "CLAUDE.md", "## Commit\n\nclaude root rule\n")
	writeFile(t, root, "GEMINI.md", "## Commit\n\ngemini root rule\n")
	writeFile(t, root, "AGENTS.md", "## Commit\n\nagents root rule\n")
	writeFile(t, root, "pkg/AGENTS.md", "## Commit\n\npkg rule\n")
	writeFile(t, root, "pkg/api/AGENTS.md", "## Commit\n\npkg api rule\n")

	docs, err := rules.DiscoverAgentDocs(root, []string{"pkg/api/handler.go"}, cfg)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"CLAUDE.md", "GEMINI.md", "AGENTS.md", "pkg/AGENTS.md", "pkg/api/AGENTS.md"}
	if len(docs) != len(want) {
		t.Fatalf("DiscoverAgentDocs() returned %d docs, want %d: %+v", len(docs), len(want), docs)
	}
	for i := range want {
		if docs[i].Path != want[i] {
			t.Errorf("docs[%d].Path = %q, want %q", i, docs[i].Path, want[i])
		}
	}
}

func TestDiscoverAgentDocsReadsClaudeAndGeminiAtTheRootOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pkg/CLAUDE.md", "## Commit\n\nnested claude\n")
	writeFile(t, root, "pkg/AGENTS.md", "## Commit\n\nnested agents\n")

	docs, err := rules.DiscoverAgentDocs(root, []string{"pkg/x.go"}, rules.DefaultConfig().Agents)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Path != "pkg/AGENTS.md" {
		t.Errorf("DiscoverAgentDocs() = %+v, want only pkg/AGENTS.md", docs)
	}
}

func TestDiscoverAgentDocsVisitsEachDirectoryOnce(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pkg/AGENTS.md", "## Commit\n\nrule\n")

	docs, err := rules.DiscoverAgentDocs(root,
		[]string{"pkg/a.go", "pkg/b.go", "pkg/c.go"}, rules.DefaultConfig().Agents)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Errorf("DiscoverAgentDocs() = %+v, want one entry despite three staged files in that directory", docs)
	}
}

func TestDiscoverAgentDocsSkipsFilesWithNothingToContribute(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# Build\n\n"+strings.Repeat("filler\n", 500))

	docs, err := rules.DiscoverAgentDocs(root, []string{"a.go"}, rules.DefaultConfig().Agents)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Errorf("DiscoverAgentDocs() = %+v, want no docs when extraction yields nothing", docs)
	}
}

func TestDiscoverAgentDocsDisabled(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "## Commit\n\nrule\n")

	cfg := rules.DefaultConfig().Agents
	cfg.Enabled = false

	docs, err := rules.DiscoverAgentDocs(root, []string{"a.go"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Errorf("DiscoverAgentDocs() = %+v, want none when agents.enabled is false", docs)
	}
}

func TestDiscoverAgentDocsIgnoresPathsOutsideTheRepository(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "## Commit\n\nrule\n")

	docs, err := rules.DiscoverAgentDocs(root, []string{"../../etc/passwd"}, rules.DefaultConfig().Agents)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Path != "AGENTS.md" {
		t.Errorf("DiscoverAgentDocs() = %+v; a path escaping the root must not widen the walk", docs)
	}
}
