package prompt_test

import (
	"strings"
	"testing"

	"github.com/guerrero/gitia/internal/commit"
	"github.com/guerrero/gitia/internal/prompt"
	"github.com/guerrero/gitia/internal/rules"
)

func TestSystemStatesTheRules(t *testing.T) {
	rs := rules.Conventional()
	rs.Types = []string{"feat", "fix"}
	rs.HeaderMaxLength = 60
	rs.Language = "en"

	got := prompt.System(rs, nil)

	for _, want := range []string{"feat", "fix", "60"} {
		if !strings.Contains(got, want) {
			t.Errorf("System() is missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "chore") {
		t.Errorf("System() mentions a type that is not allowed\n%s", got)
	}
}

func TestSystemOrdersAgentDocsLeastAuthoritativeFirst(t *testing.T) {
	docs := []rules.AgentDoc{
		{Path: "CLAUDE.md", Content: "claude says alpha"},
		{Path: "AGENTS.md", Content: "agents says beta"},
		{Path: "pkg/api/AGENTS.md", Content: "nested says gamma"},
	}

	got := prompt.System(rules.Conventional(), docs)

	iClaude := strings.Index(got, "claude says alpha")
	iRoot := strings.Index(got, "agents says beta")
	iNested := strings.Index(got, "nested says gamma")

	if iClaude < 0 || iRoot < 0 || iNested < 0 {
		t.Fatalf("System() dropped a document\n%s", got)
	}
	if iClaude >= iRoot || iRoot >= iNested {
		t.Errorf("System() ordered documents wrongly: claude=%d root=%d nested=%d", iClaude, iRoot, iNested)
	}
}

func TestSystemLabelsEachBlockWithItsSourcePath(t *testing.T) {
	docs := []rules.AgentDoc{{Path: "pkg/api/AGENTS.md", Content: "rule"}}

	got := prompt.System(rules.Conventional(), docs)
	if !strings.Contains(got, "pkg/api/AGENTS.md") {
		t.Errorf("System() did not label the block with its source path\n%s", got)
	}
}

func TestSystemStatesTheOverrideRule(t *testing.T) {
	docs := []rules.AgentDoc{
		{Path: "AGENTS.md", Content: "a"},
		{Path: "pkg/AGENTS.md", Content: "b"},
	}

	got := strings.ToLower(prompt.System(rules.Conventional(), docs))
	if !strings.Contains(got, "later") || !strings.Contains(got, "override") {
		t.Errorf("System() must tell the model that later blocks override earlier ones\n%s", got)
	}
}

func TestSystemSaysNoBodyWhenDisabled(t *testing.T) {
	rs := rules.Conventional()
	rs.IncludeBody = false

	got := strings.ToLower(prompt.System(rs, nil))
	if !strings.Contains(got, "no body") && !strings.Contains(got, "subject line only") {
		t.Errorf("System() did not tell the model to omit the body\n%s", got)
	}
}

func TestSystemAsksForIntentNotMechanics(t *testing.T) {
	got := strings.ToLower(prompt.System(rules.Conventional(), nil))

	if !strings.Contains(got, "intent") {
		t.Errorf("System() must ask for the change's intent\n%s", got)
	}
	if !strings.Contains(got, "disable telemetry") || !strings.Contains(got, "update settings") {
		t.Errorf("System() must contrast intent with the mechanical description\n%s", got)
	}
	if !strings.Contains(got, "hunks") {
		t.Errorf("System() must tell the model to read the diff hunks\n%s", got)
	}
	if !strings.Contains(got, "semantic change") {
		t.Errorf("System() must ask the body to describe the semantic change\n%s", got)
	}
}

func TestUserIncludesTheStatAndEveryPath(t *testing.T) {
	b := prompt.Budgeted{
		Stat: " a.go | 2 +-\n b.go | 1 +\n",
		Files: []prompt.BudgetedFile{
			{Path: "a.go", Status: "M", Patch: "diff --git a/a.go b/a.go\n+changed\n"},
			{Path: "b.go", Status: "A"},
			{Path: "new.go", Status: "R", OldPath: "old.go"},
		},
	}

	got := prompt.User(b)

	for _, want := range []string{"a.go", "b.go", "new.go", "old.go", "+changed", "2 +-"} {
		if !strings.Contains(got, want) {
			t.Errorf("User() is missing %q\n%s", want, got)
		}
	}
}

func TestUserMarksFilesWithNoPatch(t *testing.T) {
	b := prompt.Budgeted{
		Stat:     "stat",
		Degraded: true,
		Files: []prompt.BudgetedFile{
			{Path: "big.json", Status: "M"},
		},
	}

	got := strings.ToLower(prompt.User(b))
	if !strings.Contains(got, "big.json") {
		t.Errorf("User() dropped a file with no patch\n%s", got)
	}
	if !strings.Contains(got, "not shown") && !strings.Contains(got, "omitted") {
		t.Errorf("User() did not mark the missing patch as omitted\n%s", got)
	}
}

func TestRetryListsTheViolations(t *testing.T) {
	base := "the original user prompt"
	got := prompt.Retry(base, []rules.Violation{
		{Rule: "type-enum", Message: `type "feature" is not one of [feat, fix]`},
		{Rule: "subject-full-stop", Message: "subject may not end with a period"},
	})

	if !strings.HasPrefix(got, base) {
		t.Error("Retry() must keep the original prompt as its prefix")
	}
	for _, want := range []string{"type-enum", "feature", "subject-full-stop", "period"} {
		if !strings.Contains(got, want) {
			t.Errorf("Retry() is missing %q\n%s", want, got)
		}
	}
}

func TestRerollAsksForSomethingMateriallyDifferent(t *testing.T) {
	base := "the original user prompt"
	got := prompt.Reroll(base, commit.Message{
		Type: "chore", Scope: "deps", Subject: "update things",
	})

	if !strings.HasPrefix(got, base) {
		t.Error("Reroll() must keep the original prompt as its prefix")
	}
	if !strings.Contains(got, "chore(deps): update things") {
		t.Errorf("Reroll() did not include the previous message\n%s", got)
	}
	if !strings.Contains(strings.ToLower(got), "different") {
		t.Errorf("Reroll() did not ask for a materially different result\n%s", got)
	}
}

func TestRerollTemperatureClimbsAndCaps(t *testing.T) {
	tests := []struct {
		n    int
		want float64
	}{
		{0, 0.2},
		{1, 0.4},
		{2, 0.6},
		{3, 0.8},
		{4, 0.8},
		{99, 0.8},
	}
	for _, tt := range tests {
		if got := prompt.RerollTemperature(0.2, tt.n); got != tt.want {
			t.Errorf("RerollTemperature(0.2, %d) = %v, want %v", tt.n, got, tt.want)
		}
	}
}
