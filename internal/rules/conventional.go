package rules

// Conventional returns the Conventional Commits v1.0.0 baseline: the lowest
// layer of the precedence stack, used when a repository declares nothing.
//
// https://www.conventionalcommits.org/en/v1.0.0/
func Conventional() RuleSet {
	return RuleSet{
		Types: []string{
			"feat", "fix", "docs", "style", "refactor", "perf",
			"test", "build", "ci", "chore", "revert",
		},
		Scopes:             nil, // any scope
		ScopeRequired:      false,
		HeaderMaxLength:    72,
		BodyMaxLineLength:  72,
		HeaderIdealMin:     50,
		HeaderIdealMax:     55,
		TypeCase:           CaseLower,
		SubjectCase:        CaseAny,
		SubjectFullStop:    true,
		BodyLeadingBlank:   true,
		FooterLeadingBlank: true,
		IncludeBody:        true,
		SignOff:            false,
		Language:           "en",
	}
}
