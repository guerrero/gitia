// Package prompt builds the system prompt, user prompt, and JSON schema sent
// to Ollama. It depends on internal/rules, never the reverse: the schema is
// derived from the resolved rules, so "what types are legal" has one source of
// truth.
package prompt

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/guerrero/gitia/internal/git"
)

// DiffInput is everything gitia collected about the staged changes.
type DiffInput struct {
	Stat    string
	Changes []git.FileChange
	Diffs   []git.FileDiff
}

// BudgetedFile is one staged path as it will appear in the prompt. An empty
// Patch means the model sees that the file changed but not how.
type BudgetedFile struct {
	Path    string
	Status  string
	OldPath string
	Patch   string
}

// Budgeted is the result of fitting the diff into the context budget.
type Budgeted struct {
	Stat     string
	Files    []BudgetedFile
	Degraded bool
	Notes    []string
}

// Budget fits the staged diff into maxBytes. Small models degrade as context
// grows and speed degrades with it, so the stat is always included and the
// patches are shed in a fixed order: excluded paths first, then the largest
// remaining files. Every file stays listed regardless, so the model never
// invents a scope for a file it did not know about.
//
// A maxBytes of zero or less disables budgeting.
func Budget(in DiffInput, maxBytes int, exclude []string) Budgeted {
	patches := map[string]string{}
	for _, d := range in.Diffs {
		patches[d.Path] = d.Patch
	}

	out := Budgeted{Stat: in.Stat}
	for _, c := range in.Changes {
		out.Files = append(out.Files, BudgetedFile{
			Path:    c.Path,
			Status:  c.Status,
			OldPath: c.OldPath,
			Patch:   patches[c.Path],
		})
	}

	if maxBytes <= 0 || total(out.Files) <= maxBytes {
		return out
	}

	// Step 1: excluded paths drop to stat-only.
	for i := range out.Files {
		if out.Files[i].Patch != "" && MatchesAny(out.Files[i].Path, exclude) {
			out.Files[i].Patch = ""
			out.Degraded = true
			out.Notes = append(out.Notes,
				fmt.Sprintf("dropped patch for %s (matches diff.exclude)", out.Files[i].Path))
		}
	}
	if total(out.Files) <= maxBytes {
		return out
	}

	// Step 2: the largest remaining files lose their hunk bodies.
	order := make([]int, 0, len(out.Files))
	for i := range out.Files {
		if out.Files[i].Patch != "" {
			order = append(order, i)
		}
	}
	slices.SortFunc(order, func(a, b int) int {
		return len(out.Files[b].Patch) - len(out.Files[a].Patch)
	})

	for _, i := range order {
		if total(out.Files) <= maxBytes {
			break
		}
		out.Notes = append(out.Notes,
			fmt.Sprintf("dropped patch for %s (%d bytes, over diff.max_bytes)",
				out.Files[i].Path, len(out.Files[i].Patch)))
		out.Files[i].Patch = ""
		out.Degraded = true
	}

	return out
}

func total(files []BudgetedFile) int {
	n := 0
	for _, f := range files {
		n += len(f.Patch)
	}
	return n
}

var globCache = map[string]*regexp.Regexp{}

// MatchesAny reports whether path matches any glob pattern.
//
// Two departures from path.Match, both matching .gitignore semantics: ** spans
// separators so "dist/**" covers "dist/a/b.js", and a pattern containing no
// separator is matched against the base name too, so "*.lock" covers
// "src/vendor.lock".
func MatchesAny(path string, patterns []string) bool {
	base := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		base = path[i+1:]
	}

	for _, p := range patterns {
		re := globRegexp(p)
		if re.MatchString(path) {
			return true
		}
		if !strings.Contains(p, "/") && re.MatchString(base) {
			return true
		}
	}
	return false
}

// globRegexp translates a glob into an anchored regexp. Patterns come from
// config, not from user input at scale, so a small unbounded cache is fine.
func globRegexp(pattern string) *regexp.Regexp {
	if re, ok := globCache[pattern]; ok {
		return re
	}

	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
				// Consume a trailing slash so "dist/**" also matches "dist".
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")

	re := regexp.MustCompile(b.String())
	globCache[pattern] = re
	return re
}
