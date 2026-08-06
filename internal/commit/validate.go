package commit

import (
	"fmt"
	"strings"

	"github.com/guerrero/gitia/internal/rules"
)

// Validate checks m against rs and returns every violation it finds, in a
// stable order. A nil return means the message is legal.
//
// Rule names match commitlint's so a violation can be quoted back to the user
// and to the model without translation.
func Validate(m Message, rs rules.RuleSet) []rules.Violation {
	var vs []rules.Violation
	add := func(rule, format string, a ...any) {
		vs = append(vs, rules.Violation{Rule: rule, Message: fmt.Sprintf(format, a...)})
	}

	switch {
	case m.Type == "":
		add("type-empty", "type may not be empty")
	case rs.TypeCase == rules.CaseLower && m.Type != strings.ToLower(m.Type):
		add("type-case", "type %q must be lower-case", m.Type)
	case !rs.AllowsType(m.Type):
		add("type-enum", "type %q is not one of [%s]", m.Type, strings.Join(rs.Types, ", "))
	}

	if rs.ScopeRequired && m.Scope == "" {
		add("scope-empty", "scope is required")
	}
	if !rs.AllowsScope(m.Scope) {
		add("scope-enum", "scope %q is not one of [%s]", m.Scope, strings.Join(rs.Scopes, ", "))
	}

	subject := strings.TrimSpace(m.Subject)
	switch subject {
	case "":
		add("subject-empty", "subject may not be empty")
	default:
		if rs.SubjectCase == rules.CaseLower && subject != strings.ToLower(subject) {
			add("subject-case", "subject must be lower-case")
		}
		if rs.SubjectFullStop && strings.HasSuffix(subject, ".") {
			add("subject-full-stop", "subject may not end with a period")
		}
	}

	if n := len(Header(m)); rs.HeaderMaxLength > 0 && n > rs.HeaderMaxLength {
		add("header-max-length", "header is %d characters, limit is %d", n, rs.HeaderMaxLength)
	}

	if rs.BodyMaxLineLength > 0 {
		for _, p := range m.Body {
			for _, line := range Wrap(p, rs.BodyMaxLineLength) {
				if len(line) > rs.BodyMaxLineLength {
					add("body-max-line-length", "body line is %d characters, limit is %d",
						len(line), rs.BodyMaxLineLength)
					break
				}
			}
		}
	}

	if m.Breaking && strings.TrimSpace(m.BreakingDescription) == "" {
		add("breaking-description-empty", "a breaking change requires a BREAKING CHANGE description")
	}

	return vs
}
