package commit

import (
	"strings"

	"github.com/guerrero/gitia/internal/rules"
)

// Repair applies every fix that is purely mechanical: case, whitespace, a
// duplicated type prefix, a trailing period, an overlong header. It never
// guesses semantics — an unknown type or an empty subject survives untouched
// so that Validate still reports it and the caller can fall back to a model
// retry. Repair is idempotent.
func Repair(m Message, rs rules.RuleSet) Message {
	m.Type = strings.TrimSpace(m.Type)
	m.Scope = strings.TrimSpace(m.Scope)
	m.Subject = strings.TrimSpace(m.Subject)

	if rs.TypeCase == rules.CaseLower {
		m.Type = strings.ToLower(m.Type)
	}

	// Models sometimes emit the full header in the subject field.
	for _, prefix := range []string{
		m.Type + ": ", m.Type + ":",
		Header(m) + " ", Header(m),
	} {
		if prefix != "" && strings.HasPrefix(m.Subject, prefix) {
			m.Subject = strings.TrimSpace(strings.TrimPrefix(m.Subject, prefix))
			break
		}
	}

	if rs.SubjectCase == rules.CaseLower {
		m.Subject = strings.ToLower(m.Subject)
	}
	if rs.SubjectFullStop {
		m.Subject = strings.TrimRight(m.Subject, ".")
		m.Subject = strings.TrimSpace(m.Subject)
	}

	if rs.HeaderMaxLength > 0 {
		m.Subject = truncateSubject(m, rs.HeaderMaxLength)
	}

	body := make([]string, 0, len(m.Body))
	for _, p := range m.Body {
		if p = strings.TrimSpace(p); p != "" {
			body = append(body, strings.Join(strings.Fields(p), " "))
		}
	}
	if len(body) == 0 {
		body = nil
	}
	m.Body = body

	return m
}

// truncateSubject drops whole trailing words until the rendered header fits.
// A single word longer than the budget is left alone: silently mangling an
// identifier is worse than one violation the caller can retry on.
func truncateSubject(m Message, limit int) string {
	if len(Header(m)) <= limit {
		return m.Subject
	}

	words := strings.Fields(m.Subject)
	for len(words) > 1 {
		words = words[:len(words)-1]
		candidate := m
		candidate.Subject = strings.Join(words, " ")
		if len(Header(candidate)) <= limit {
			return strings.TrimRight(candidate.Subject, ".")
		}
	}
	return m.Subject
}
