// Package commit models a conventional commit message and validates one
// against a rules.RuleSet it is handed. It knows nothing about where those
// rules came from.
package commit

// Footer is one git trailer, rendered as "Token: Value".
type Footer struct {
	Token string
	Value string
}

// Message is a structured conventional commit. Body is a slice of paragraphs
// rather than one string so that rewrapping to the rule width is a
// deterministic post-process instead of something the model has to get right.
type Message struct {
	Type                string
	Scope               string
	Subject             string
	Body                []string
	Breaking            bool
	BreakingDescription string
	Footers             []Footer
}
