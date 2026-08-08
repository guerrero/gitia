package prompt

import (
	"fmt"
	"math"
	"strings"

	"github.com/guerrero/gitia/internal/commit"
	"github.com/guerrero/gitia/internal/rules"
)

// maxRerollTemperature caps how far regeneration is allowed to wander. Past
// this point the model stops writing commit messages and starts writing prose.
const maxRerollTemperature = 0.8

// System builds the system prompt: the resolved rules, then the repository's
// own conventions in ascending order of authority.
func System(rs rules.RuleSet, docs []rules.AgentDoc) string {
	var b strings.Builder

	b.WriteString("You write git commit messages in the Conventional Commits format.\n")
	b.WriteString("You are given a staged diff. Reply with JSON matching the provided schema and nothing else.\n\n")

	b.WriteString("Rules:\n")
	fmt.Fprintf(&b, "- The type must be exactly one of: %s\n", strings.Join(rs.Types, ", "))
	if len(rs.Scopes) > 0 {
		fmt.Fprintf(&b, "- The scope must be exactly one of: %s\n", strings.Join(rs.Scopes, ", "))
	} else {
		b.WriteString("- The scope is optional. Use the package or directory the change belongs to, or omit it.\n")
	}
	if rs.ScopeRequired {
		b.WriteString("- A scope is required.\n")
	}
	if rs.HeaderMaxLength > 0 {
		fmt.Fprintf(&b, "- The whole \"type(scope): subject\" line must be at most %d characters.\n", rs.HeaderMaxLength)
	}
	if rs.HeaderIdealMin > 0 && rs.HeaderIdealMax > 0 && rs.HeaderIdealMin <= rs.HeaderIdealMax {
		idealMax := rs.HeaderIdealMax
		if rs.HeaderMaxLength > 0 && idealMax > rs.HeaderMaxLength {
			idealMax = rs.HeaderMaxLength
		}
		if rs.HeaderIdealMin <= idealMax {
			fmt.Fprintf(&b, "- Prefer a header of %d–%d characters; the limit above is a ceiling, not a target.\n",
				rs.HeaderIdealMin, idealMax)
		}
	}
	b.WriteString("- Write the subject in the imperative mood: \"add\", not \"added\" or \"adds\".\n")
	b.WriteString("- The subject names the intent of the change — what it accomplishes for a user of the code — not the mechanics of the diff. Prefer \"disable telemetry\" over \"update settings\".")
	b.WriteString("\n")
	b.WriteString("- Infer that intent from the diff hunks, not from the file list: the stat and the paths say what was touched, not what changed.\n")
	if rs.SubjectFullStop {
		b.WriteString("- Do not end the subject with a period.\n")
	}
	if rs.SubjectCase == rules.CaseLower {
		b.WriteString("- Write the subject in lower case.\n")
	}
	if rs.IncludeBody {
		b.WriteString("- Write a short body explaining why the change was made, not what the diff already shows.\n")
		b.WriteString("- When the subject alone does not tell the whole story, the body describes the semantic change: what the code now does that it did not before.\n")
		b.WriteString("- Each body entry is one paragraph. Do not wrap lines yourself.\n")
		b.WriteString("- Omit the body entirely when the subject says everything.\n")
	} else {
		b.WriteString("- Write no body: the subject line only.\n")
	}
	b.WriteString("- Set breaking to true only when the change breaks an existing interface.\n")
	if rs.Language != "" && rs.Language != "en" {
		fmt.Fprintf(&b, "- Write the message in this language: %s\n", rs.Language)
	}

	if len(docs) > 0 {
		b.WriteString("\nThis repository declares its own conventions below.\n")
		b.WriteString("They are ordered least to most authoritative: where two blocks conflict, ")
		b.WriteString("the later block overrides the earlier one.\n")
		for _, d := range docs {
			fmt.Fprintf(&b, "\n--- from %s ---\n%s", d.Path, strings.TrimRight(d.Content, "\n"))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// User builds the user prompt from the budgeted diff. The stat is always
// present, and every changed path is listed even when its patch was dropped,
// so the model never invents a scope for a file it did not know about.
func User(b Budgeted) string {
	var s strings.Builder

	s.WriteString("Staged changes:\n\n")
	s.WriteString(strings.TrimRight(b.Stat, "\n"))
	s.WriteString("\n\nFiles:\n")

	for _, f := range b.Files {
		switch {
		case f.OldPath != "":
			fmt.Fprintf(&s, "- %s: %s -> %s\n", statusWord(f.Status), f.OldPath, f.Path)
		default:
			fmt.Fprintf(&s, "- %s: %s\n", statusWord(f.Status), f.Path)
		}
	}

	for _, f := range b.Files {
		if f.Patch == "" {
			continue
		}
		s.WriteString("\n")
		s.WriteString(strings.TrimRight(f.Patch, "\n"))
		s.WriteString("\n")
	}

	var omitted []string
	for _, f := range b.Files {
		if f.Patch == "" {
			omitted = append(omitted, f.Path)
		}
	}
	if len(omitted) > 0 {
		fmt.Fprintf(&s, "\nThe contents of these files are not shown, but they did change: %s\n",
			strings.Join(omitted, ", "))
	}

	return s.String()
}

func statusWord(status string) string {
	switch status {
	case "A":
		return "added"
	case "M":
		return "modified"
	case "D":
		return "deleted"
	case "R":
		return "renamed"
	case "C":
		return "copied"
	case "T":
		return "type changed"
	default:
		return "changed"
	}
}

// Retry appends the validation failures to the user prompt so the model can
// correct them, rather than re-rolling blind.
func Retry(base string, violations []rules.Violation) string {
	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\nYour previous answer was rejected for these reasons. Fix all of them:\n")
	for _, v := range violations {
		fmt.Fprintf(&b, "- %s: %s\n", v.Rule, v.Message)
	}
	return b.String()
}

// Reroll appends the previous message with an instruction to produce something
// materially different. Re-rolls are the primary escape hatch when the model
// picks a poor type or a vague subject.
func Reroll(base string, previous commit.Message) string {
	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\nYou already produced this message:\n\n")
	b.WriteString(commit.Header(previous))
	b.WriteString("\n")
	for _, p := range previous.Body {
		b.WriteString("\n")
		b.WriteString(p)
		b.WriteString("\n")
	}
	b.WriteString("\nProduce a materially different message: a different angle on what changed, ")
	b.WriteString("or a different type or scope if one fits better. Do not paraphrase the above.\n")
	return b.String()
}

// RerollTemperature raises the temperature by 0.2 per re-roll, to a ceiling of
// 0.8.
func RerollTemperature(base float64, n int) float64 {
	t := base + 0.2*float64(n)
	t = math.Round(t*100) / 100
	if t > maxRerollTemperature {
		return maxRerollTemperature
	}
	return t
}
