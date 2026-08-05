package ui

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/guerrero/gitia/internal/exitcode"
)

// Editor resolves the editor command: $EDITOR, then $VISUAL, then vi. The
// value is split into command and arguments, honoring single- and double-quote
// grouping so both "code --wait" and "sh -c '...' --" work.
func Editor() []string {
	for _, env := range []string{"EDITOR", "VISUAL"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			if fields := splitCommand(v); len(fields) > 0 {
				return fields
			}
		}
	}
	return []string{"vi"}
}

// splitCommand splits a shell-like command line into tokens, grouping words
// inside single quotes (no escapes) and double quotes (backslash escapes), the
// way a shell would before running it. It never fails; stray quotes are kept
// literally, which matches the tolerant behavior of $EDITOR handling.
func splitCommand(s string) []string {
	var fields []string
	var cur strings.Builder
	inSingle, inDouble := false, false

	flush := func() {
		if cur.Len() > 0 {
			fields = append(fields, cur.String())
			cur.Reset()
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(c)
			}
		case inDouble:
			switch c {
			case '"':
				inDouble = false
			case '\\':
				if i+1 < len(s) {
					i++
					cur.WriteByte(s[i])
				}
			default:
				cur.WriteByte(c)
			}
		default:
			switch c {
			case '\'':
				inSingle = true
			case '"':
				inDouble = true
			case ' ', '\t', '\n', '\r':
				flush()
			default:
				cur.WriteByte(c)
			}
		}
	}
	flush()
	return fields
}

// EditMessage writes message to a temp file, opens the editor on it, and
// returns the saved result with comment lines stripped. An empty buffer aborts,
// matching git's own behavior.
func EditMessage(ctx context.Context, message string) (string, error) {
	f, err := os.CreateTemp("", "gitia-*.gitcommit")
	if err != nil {
		return "", err
	}
	path := f.Name()
	defer os.Remove(path)

	if _, err := f.WriteString(message); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	argv := append(Editor(), path)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", exitcode.Wrapf(exitcode.Generic, "editor %s: %v", argv[0], err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	edited := stripComments(string(data))
	if edited == "" {
		return "", exitcode.Wrapf(exitcode.Aborted, "aborting due to empty commit message")
	}
	return edited, nil
}

// stripComments removes lines beginning with # and trims surrounding blank
// lines, the way git does when it reads an edited COMMIT_EDITMSG.
func stripComments(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}
