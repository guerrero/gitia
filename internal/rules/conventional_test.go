package rules_test

import (
	"testing"

	"github.com/guerrero/gitia/internal/rules"
)

func TestConventionalBaseline(t *testing.T) {
	rs := rules.Conventional()

	if got, want := rs.HeaderMaxLength, 72; got != want {
		t.Errorf("HeaderMaxLength = %d, want %d", got, want)
	}
	if got, want := rs.BodyMaxLineLength, 72; got != want {
		t.Errorf("BodyMaxLineLength = %d, want %d", got, want)
	}
	if got, want := rs.HeaderIdealMin, 50; got != want {
		t.Errorf("HeaderIdealMin = %d, want %d", got, want)
	}
	if got, want := rs.HeaderIdealMax, 55; got != want {
		t.Errorf("HeaderIdealMax = %d, want %d", got, want)
	}
	if rs.TypeCase != rules.CaseLower {
		t.Errorf("TypeCase = %q, want %q", rs.TypeCase, rules.CaseLower)
	}
	if !rs.SubjectFullStop {
		t.Error("SubjectFullStop = false, want true (subject must not end with a period)")
	}
	if !rs.BodyLeadingBlank {
		t.Error("BodyLeadingBlank = false, want true")
	}
	if rs.ScopeRequired {
		t.Error("ScopeRequired = true, want false (scope is optional in v1.0.0)")
	}
	if len(rs.Scopes) != 0 {
		t.Errorf("Scopes = %v, want empty (any scope allowed)", rs.Scopes)
	}
	if rs.Language != "en" {
		t.Errorf("Language = %q, want \"en\"", rs.Language)
	}

	want := []string{
		"feat", "fix", "docs", "style", "refactor", "perf",
		"test", "build", "ci", "chore", "revert",
	}
	if len(rs.Types) != len(want) {
		t.Fatalf("Types = %v, want %v", rs.Types, want)
	}
	for i := range want {
		if rs.Types[i] != want[i] {
			t.Errorf("Types[%d] = %q, want %q", i, rs.Types[i], want[i])
		}
	}
}

func TestAllowsType(t *testing.T) {
	rs := rules.Conventional()

	tests := []struct {
		typ  string
		want bool
	}{
		{"feat", true},
		{"fix", true},
		{"revert", true},
		{"feature", false},
		{"Feat", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := rs.AllowsType(tt.typ); got != tt.want {
			t.Errorf("AllowsType(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestAllowsScopeEmptyListAllowsAnything(t *testing.T) {
	rs := rules.Conventional()
	for _, s := range []string{"", "api", "anything-at-all"} {
		if !rs.AllowsScope(s) {
			t.Errorf("AllowsScope(%q) = false, want true when Scopes is empty", s)
		}
	}
}

func TestAllowsScopeRespectsAllowList(t *testing.T) {
	rs := rules.Conventional()
	rs.Scopes = []string{"api", "cli"}

	if !rs.AllowsScope("api") {
		t.Error("AllowsScope(\"api\") = false, want true")
	}
	if rs.AllowsScope("db") {
		t.Error("AllowsScope(\"db\") = true, want false")
	}
	if !rs.AllowsScope("") {
		t.Error("AllowsScope(\"\") = false; an empty scope is allowed unless ScopeRequired")
	}
}

func TestCloneDoesNotAliasSlices(t *testing.T) {
	rs := rules.Conventional()
	clone := rs.Clone()
	clone.Types[0] = "MUTATED"

	if rs.Types[0] == "MUTATED" {
		t.Error("Clone aliased the Types slice; mutating the clone changed the original")
	}
}
