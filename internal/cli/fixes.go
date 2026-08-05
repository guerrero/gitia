package cli

import "strings"

// PreparseFixes rewrites the `--fixes 123 456` form, which POSIX flag parsing
// cannot express, into the comma-separated form pflag understands. It runs
// over os.Args before cobra sees them.
//
// Bare numeric arguments immediately following --fixes/-f are absorbed
// greedily; the first non-numeric argument terminates the absorption. A
// leading # is tolerated and stripped.
func PreparseFixes(args []string) []string {
	out := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		out = append(out, arg)

		if arg == "--" {
			// Everything after a bare double dash belongs to the caller.
			out = append(out, args[i+1:]...)
			return out
		}

		inline, hasInline := fixesInlineValue(arg)
		if !isFixesFlag(arg) && !hasInline {
			continue
		}

		values := []string{}
		if hasInline {
			values = append(values, inline)
		} else if i+1 < len(args) {
			// The immediate value, numeric or not, belongs to the flag.
			i++
			out = append(out, strings.TrimPrefix(args[i], "#"))
			values = nil
		}

		// Absorb the bare numeric arguments that follow.
		var absorbed []string
		for i+1 < len(args) && isIssueNumber(args[i+1]) {
			i++
			absorbed = append(absorbed, strings.TrimPrefix(args[i], "#"))
		}
		if len(absorbed) == 0 {
			continue
		}

		last := len(out) - 1
		if hasInline {
			out[last] = fixesFlagName(arg) + "=" + strings.Join(append(values, absorbed...), ",")
			continue
		}
		out[last] = strings.Join(append([]string{out[last]}, absorbed...), ",")
	}

	return out
}

// isFixesFlag reports whether arg is the long form, or a short cluster ending
// in f so that -nf takes a value the way pflag expects.
func isFixesFlag(arg string) bool {
	if arg == "--fixes" {
		return true
	}
	if len(arg) >= 2 && arg[0] == '-' && arg[1] != '-' {
		return strings.HasSuffix(arg, "f")
	}
	return false
}

// fixesInlineValue splits the --fixes=123 form.
func fixesInlineValue(arg string) (string, bool) {
	if v, ok := strings.CutPrefix(arg, "--fixes="); ok {
		return v, true
	}
	return "", false
}

func fixesFlagName(arg string) string {
	if strings.HasPrefix(arg, "--fixes=") {
		return "--fixes"
	}
	return arg
}

// isIssueNumber reports whether arg is a bare issue number, with or without a
// leading #.
func isIssueNumber(arg string) bool {
	s := strings.TrimPrefix(arg, "#")
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
