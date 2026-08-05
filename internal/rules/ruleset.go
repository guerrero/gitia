// Package rules resolves the style rules that govern a generated commit
// message. It is the only package that knows about precedence: every other
// package is handed a finished RuleSet and asks no questions about its origin.
package rules

import "slices"

// Case constrains the letter case of a message component.
type Case string

const (
	// CaseLower requires the value to equal its lowercase form.
	CaseLower Case = "lower-case"
	// CaseAny imposes no constraint.
	CaseAny Case = "any"
)

// RuleSet is the fully resolved style contract for one commit message.
type RuleSet struct {
	// Types is the allow-list of commit types, in preference order.
	Types []string
	// Scopes is an allow-list of scopes. Empty means any scope is permitted.
	Scopes []string
	// ScopeRequired rejects a message with no scope.
	ScopeRequired bool

	// HeaderMaxLength bounds the whole "type(scope)!: subject" line, counted
	// the way commitlint counts it. Zero disables the check.
	HeaderMaxLength int
	// BodyMaxLineLength bounds each rendered body line. Zero disables the check.
	BodyMaxLineLength int

	TypeCase    Case
	SubjectCase Case

	// SubjectFullStop true means the subject must NOT end with a period.
	// The field is named for commitlint's subject-full-stop rule, which is
	// conventionally configured [2, "never", "."]; the polarity is inverted
	// relative to the field name on purpose so the two line up.
	SubjectFullStop bool

	// BodyLeadingBlank requires a blank line between header and body.
	BodyLeadingBlank bool
	// FooterLeadingBlank requires a blank line between body and footers.
	FooterLeadingBlank bool

	// IncludeBody asks the model for a body at all.
	IncludeBody bool
	// SignOff appends a Signed-off-by trailer via `git commit -s`.
	SignOff bool
	// Language is the natural language the message is written in.
	Language string
}

// Violation is one failed rule, named so the message can quote it back.
type Violation struct {
	Rule    string
	Message string
}

// AllowsType reports whether t is in the allow-list. An empty Types list
// allows nothing: a RuleSet with no legal types is a configuration error and
// callers surface it as one.
func (r RuleSet) AllowsType(t string) bool {
	return slices.Contains(r.Types, t)
}

// AllowsScope reports whether s is permitted. An empty Scopes list permits any
// scope, including none; ScopeRequired is checked separately by the validator.
func (r RuleSet) AllowsScope(s string) bool {
	if len(r.Scopes) == 0 || s == "" {
		return true
	}
	return slices.Contains(r.Scopes, s)
}

// Clone returns a copy whose slice fields share no backing array with r.
func (r RuleSet) Clone() RuleSet {
	out := r
	out.Types = slices.Clone(r.Types)
	out.Scopes = slices.Clone(r.Scopes)
	return out
}
