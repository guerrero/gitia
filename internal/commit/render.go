package commit

import (
	"strings"

	"github.com/guerrero/gitia/internal/rules"
)

const breakingToken = "BREAKING CHANGE"

// Header renders the first line: "type(scope)!: subject".
func Header(m Message) string {
	var b strings.Builder
	b.WriteString(m.Type)
	if m.Scope != "" {
		b.WriteString("(")
		b.WriteString(m.Scope)
		b.WriteString(")")
	}
	if m.Breaking {
		b.WriteString("!")
	}
	b.WriteString(": ")
	b.WriteString(m.Subject)
	return b.String()
}

// Render produces the full commit text, terminated by exactly one newline.
func Render(m Message, rs rules.RuleSet) string {
	blocks := []string{Header(m)}

	if rs.IncludeBody {
		for _, p := range m.Body {
			if strings.TrimSpace(p) == "" {
				continue
			}
			blocks = append(blocks, strings.Join(Wrap(p, rs.BodyMaxLineLength), "\n"))
		}
	}

	var footers []string
	if m.Breaking && m.BreakingDescription != "" {
		footers = append(footers, breakingToken+": "+m.BreakingDescription)
	}
	for _, f := range m.Footers {
		if f.Token == "" || f.Value == "" {
			continue
		}
		footers = append(footers, f.Token+": "+f.Value)
	}
	if len(footers) > 0 {
		blocks = append(blocks, strings.Join(footers, "\n"))
	}

	return strings.Join(blocks, "\n\n") + "\n"
}

// Wrap greedily wraps one paragraph to width columns. A token longer than
// width is left intact on its own line rather than broken: URLs and
// identifiers must survive round-tripping. A width of zero or less disables
// wrapping.
func Wrap(paragraph string, width int) []string {
	paragraph = strings.Join(strings.Fields(paragraph), " ")
	if width <= 0 || paragraph == "" {
		return []string{paragraph}
	}

	var lines []string
	var line strings.Builder

	for _, word := range strings.Fields(paragraph) {
		switch {
		case line.Len() == 0:
			line.WriteString(word)
		case line.Len()+1+len(word) <= width:
			line.WriteString(" ")
			line.WriteString(word)
		default:
			lines = append(lines, line.String())
			line.Reset()
			line.WriteString(word)
		}
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return lines
}
