package rules

import "fmt"

// FlagOverrides is layer 5: explicit intent from the command line.
type FlagOverrides struct {
	Type     string
	Scope    string
	NoBody   bool
	Breaking bool
	Model    string
}

// Layers is every input to the precedence engine. Constraints is nil when
// commitlint is absent or disabled.
type Layers struct {
	Config      Config
	AgentDocs   []AgentDoc
	Flags       FlagOverrides
	Constraints *Constraints
}

// Resolved is the finished contract every downstream package is handed.
type Resolved struct {
	// Rules is the merged, gated style contract.
	Rules RuleSet
	// AgentDocs is the prose layer, ordered least authoritative first.
	AgentDocs []AgentDoc
	// Overrides lists the preferences commitlint overruled.
	Overrides []Override
}

// Resolve merges every layer, lowest to highest, then applies commitlint as a
// hard validity gate across all of them.
//
// Layers 3 and 4 — the AGENTS.md family — govern style as prose rather than as
// structured rules, so they are carried through to the prompt in order rather
// than merged into the RuleSet. The system prompt states that later blocks
// override earlier ones.
func Resolve(l Layers) Resolved {
	rs := l.Config.Apply(Conventional()) // layers 1 and 2

	// Layer 5: explicit flags. --type narrows the enum to a single member so
	// the constrained decode literally cannot produce anything else.
	if l.Flags.Type != "" {
		rs.Types = []string{l.Flags.Type}
	}
	if l.Flags.Scope != "" {
		rs.Scopes = []string{l.Flags.Scope}
		rs.ScopeRequired = true
	}
	if l.Flags.NoBody {
		rs.IncludeBody = false
	}

	var overrides []Override
	if l.Constraints != nil {
		rs, overrides = l.Constraints.Apply(rs)
	}

	return Resolved{Rules: rs, AgentDocs: l.AgentDocs, Overrides: overrides}
}

// Warnings renders one line per overruled preference, naming the commitlint
// rule responsible so the user can see why their AGENTS.md was not honored.
func (r Resolved) Warnings() []string {
	out := make([]string, 0, len(r.Overrides))
	for _, o := range r.Overrides {
		out = append(out, fmt.Sprintf(
			"warning: commitlint %s overrode AGENTS.md preference %q → %q", o.Rule, o.From, o.To))
	}
	return out
}
