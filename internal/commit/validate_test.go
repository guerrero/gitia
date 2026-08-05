package commit_test

import (
	"strings"
	"testing"

	"github.com/guerrero/gitia/internal/commit"
	"github.com/guerrero/gitia/internal/rules"
)

func rulesFor(t *testing.T) rules.RuleSet {
	t.Helper()
	return rules.Conventional()
}

func hasRule(vs []rules.Violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

func TestValidateAcceptsAGoodMessage(t *testing.T) {
	got := commit.Validate(commit.Message{
		Type:    "feat",
		Scope:   "commit",
		Subject: "generate messages from the staged diff",
		Body:    []string{"Short body."},
	}, rulesFor(t))

	if len(got) != 0 {
		t.Errorf("Validate() = %v, want no violations", got)
	}
}

func TestValidateRules(t *testing.T) {
	rs := rulesFor(t)
	rs.Scopes = []string{"cli", "git"}

	tests := []struct {
		name     string
		msg      commit.Message
		wantRule string
	}{
		{"unknown type", commit.Message{Type: "feature", Subject: "x"}, "type-enum"},
		{"uppercase type", commit.Message{Type: "Feat", Subject: "x"}, "type-case"},
		{"empty type", commit.Message{Subject: "x"}, "type-empty"},
		{"empty subject", commit.Message{Type: "feat"}, "subject-empty"},
		{"subject ends with a period", commit.Message{Type: "feat", Subject: "does a thing."}, "subject-full-stop"},
		{"scope not in allow-list", commit.Message{Type: "feat", Scope: "db", Subject: "x"}, "scope-enum"},
		{"header too long", commit.Message{Type: "feat", Subject: strings.Repeat("x", 80)}, "header-max-length"},
		{"body line too long", commit.Message{Type: "feat", Subject: "x", Body: []string{strings.Repeat("y", 120)}}, "body-max-line-length"},
		{"breaking without a description", commit.Message{Type: "feat", Subject: "x", Breaking: true}, "breaking-description-empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commit.Validate(tt.msg, rs)
			if !hasRule(got, tt.wantRule) {
				t.Errorf("Validate() = %v, want a %q violation", got, tt.wantRule)
			}
		})
	}
}

func TestValidateBodyLengthMeasuredAfterWrapping(t *testing.T) {
	rs := rulesFor(t)
	// 120 characters of ordinary words wraps cleanly to 100, so this is legal.
	body := strings.TrimSpace(strings.Repeat("word ", 30))

	if got := commit.Validate(commit.Message{
		Type: "feat", Subject: "x", Body: []string{body},
	}, rs); hasRule(got, "body-max-line-length") {
		t.Errorf("Validate() = %v; a wrappable paragraph must not violate body-max-line-length", got)
	}
}

func TestValidateScopeRequired(t *testing.T) {
	rs := rulesFor(t)
	rs.ScopeRequired = true

	if got := commit.Validate(commit.Message{Type: "feat", Subject: "x"}, rs); !hasRule(got, "scope-empty") {
		t.Errorf("Validate() = %v, want a scope-empty violation", got)
	}
}

func TestValidateZeroLengthsDisableTheCheck(t *testing.T) {
	rs := rulesFor(t)
	rs.HeaderMaxLength = 0
	rs.BodyMaxLineLength = 0

	got := commit.Validate(commit.Message{
		Type: "feat", Subject: strings.Repeat("x", 300),
		Body: []string{strings.Repeat("y", 300)},
	}, rs)

	if hasRule(got, "header-max-length") || hasRule(got, "body-max-line-length") {
		t.Errorf("Validate() = %v; zero must disable the length checks", got)
	}
}
