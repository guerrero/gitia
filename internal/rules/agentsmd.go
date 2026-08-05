package rules

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// AgentDoc is one discovered convention file, reduced to the sections that
// bear on writing a commit message.
type AgentDoc struct {
	// Path is repository-relative and always uses forward slashes.
	Path string
	// Content is the extracted sections, or the whole file when it is small
	// and has no matching heading.
	Content string
}

// headingKeywords select the sections worth sending to the model. These files
// are typically long — build commands, test setup, code style — and mostly
// irrelevant to a commit message; feeding all of it to a 2B model wastes
// context and distracts it.
var headingKeywords = []string{"commit", "git", "changelog", "message", "versioning"}

// DiscoverAgentDocs collects convention files covering the staged paths and
// returns them ordered least authoritative first. Because these files are
// prose, "nearest wins" cannot mean structured merging; it is implemented as
// prompt ordering, and the system prompt tells the model that later blocks
// override earlier ones.
func DiscoverAgentDocs(repoRoot string, stagedPaths []string, cfg AgentsConfig) ([]AgentDoc, error) {
	if !cfg.Enabled || len(cfg.Files) == 0 {
		return nil, nil
	}

	allowed := map[string]bool{}
	for _, f := range cfg.Files {
		allowed[f] = true
	}

	var docs []AgentDoc
	seen := map[string]bool{}

	add := func(rel string) error {
		if seen[rel] {
			return nil
		}
		seen[rel] = true

		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if content := ExtractSections(string(data), cfg.MaxBytes); content != "" {
			docs = append(docs, AgentDoc{Path: rel, Content: content})
		}
		return nil
	}

	// Root CLAUDE.md and GEMINI.md first, then root AGENTS.md: AGENTS.md wins
	// conflicts when several exist at the root.
	for _, name := range []string{"CLAUDE.md", "GEMINI.md", "AGENTS.md"} {
		if !allowed[name] {
			continue
		}
		if err := add(name); err != nil {
			return nil, err
		}
	}

	if !allowed["AGENTS.md"] {
		return docs, nil
	}

	// Then nested AGENTS.md, root-to-leaf, so the deepest file is last and
	// therefore most authoritative.
	for _, dir := range stagedDirs(stagedPaths) {
		if dir == "." {
			continue
		}
		if err := add(path.Join(dir, "AGENTS.md")); err != nil {
			return nil, err
		}
	}
	return docs, nil
}

// stagedDirs returns every ancestor directory of the staged paths, shallowest
// first, deduplicated. Paths that escape the repository root are dropped.
func stagedDirs(stagedPaths []string) []string {
	seen := map[string]bool{}
	var byDepth [][]string

	for _, p := range stagedPaths {
		p = path.Clean(filepath.ToSlash(p))
		if p == "." || strings.HasPrefix(p, "../") || path.IsAbs(p) {
			continue
		}
		parts := strings.Split(path.Dir(p), "/")
		for i := range parts {
			dir := path.Join(parts[:i+1]...)
			if dir == "." || dir == "" || seen[dir] {
				continue
			}
			seen[dir] = true
			depth := strings.Count(dir, "/")
			for len(byDepth) <= depth {
				byDepth = append(byDepth, nil)
			}
			byDepth[depth] = append(byDepth[depth], dir)
		}
	}

	var out []string
	for _, level := range byDepth {
		out = append(out, level...)
	}
	return out
}

// ExtractSections returns the Markdown sections whose heading text mentions a
// commit-related keyword. Nested subsections come along with their parent. If
// nothing matches and the file is at most maxBytes, the whole file is returned
// so that repositories writing their conventions in prose are not silently
// ignored; a larger unmatched file yields the empty string.
//
// There is deliberately no gitia-specific marker syntax: AGENTS.md stays
// vendor-neutral.
func ExtractSections(markdown string, maxBytes int) string {
	lines := strings.Split(markdown, "\n")

	var out []string
	includeDepth := 0 // 0 means "not currently inside an included section"
	matched := false
	anyHeading := false
	inFence := false
	var fence string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case inFence:
			if strings.HasPrefix(trimmed, fence) {
				inFence = false
			}
		case strings.HasPrefix(trimmed, "```"), strings.HasPrefix(trimmed, "~~~"):
			inFence = true
			fence = trimmed[:3]
		}

		if depth, text, ok := parseHeading(line); ok && !inFence {
			anyHeading = true
			switch {
			case includeDepth > 0 && depth <= includeDepth:
				// A sibling or shallower heading ends the included section.
				includeDepth = 0
				fallthrough
			case includeDepth == 0:
				if headingMatches(text) {
					includeDepth = depth
					matched = true
				}
			}
		}

		if includeDepth > 0 {
			out = append(out, line)
		}
	}

	if !matched {
		// Fall back to the whole file only for pure prose with no headings at
		// all. If headings exist but none match, returning the whole file would
		// smuggle in build/test instructions that the keyword filter exists to
		// keep out — and a keyword-looking line inside a code fence is not a
		// heading.
		if !anyHeading && maxBytes > 0 && len(markdown) <= maxBytes {
			return markdown
		}
		return ""
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}

// parseHeading reports the depth and text of an ATX heading line.
func parseHeading(line string) (depth int, text string, ok bool) {
	trimmed := strings.TrimLeft(line, " ")
	if !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	depth = len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
	if depth < 1 || depth > 6 {
		return 0, "", false
	}
	rest := trimmed[depth:]
	if rest != "" && !strings.HasPrefix(rest, " ") {
		return 0, "", false // "#hashtag" is not a heading
	}
	return depth, strings.TrimSpace(rest), true
}

func headingMatches(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range headingKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
