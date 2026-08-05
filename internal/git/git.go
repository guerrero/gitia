// Package git shells out to the git binary. It carries no policy: callers
// decide what to do with what it returns.
package git

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"github.com/guerrero/gitia/internal/exitcode"
)

// FileChange is one entry from `git diff --staged --name-status`.
type FileChange struct {
	// Status is the single-letter code: A, M, D, R, C, or T.
	Status string
	// Path is the current path, repository-relative.
	Path string
	// OldPath is the previous path, set only for renames and copies.
	OldPath string
}

// FileDiff is the complete `diff --git` block for one path.
type FileDiff struct {
	Path  string
	Patch string
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", exitcode.Wrapf(exitcode.Generic, "git %s: %s", args[0], msg)
	}
	return stdout.String(), nil
}

// Version returns the output of `git --version`, trimmed.
func Version(ctx context.Context) (string, error) {
	out, err := run(ctx, "", "--version")
	return strings.TrimSpace(out), err
}

// RepoRoot returns the absolute path to the work tree containing dir.
func RepoRoot(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", exitcode.Wrapf(exitcode.NotRepo, "not a git repository (or any parent up to the mount point)")
	}
	return strings.TrimSpace(out), nil
}

// HasStagedChanges reports whether the index differs from HEAD. It reads only
// the index: gitia never sees or commits unstaged work.
func HasStagedChanges(ctx context.Context, dir string) (bool, error) {
	changes, err := StagedNameStatus(ctx, dir)
	return len(changes) > 0, err
}

// StagedStat returns `git diff --staged --stat`: cheap, high signal, and
// always included in the prompt regardless of the diff budget.
func StagedStat(ctx context.Context, dir string) (string, error) {
	return run(ctx, dir, "diff", "--staged", "--stat")
}

// StagedNameStatus lists every staged path with its change status. -z is used
// so paths containing spaces, quotes, or newlines survive intact.
func StagedNameStatus(ctx context.Context, dir string) ([]FileChange, error) {
	out, err := run(ctx, dir, "diff", "--staged", "--name-status", "-M", "-z")
	if err != nil {
		return nil, err
	}

	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	var changes []FileChange

	for i := 0; i < len(fields); {
		status := fields[i]
		if status == "" {
			i++
			continue
		}
		// Renames and copies carry a similarity score (R100) and two paths.
		letter := status[:1]
		if letter == "R" || letter == "C" {
			if i+2 >= len(fields) {
				break
			}
			changes = append(changes, FileChange{Status: letter, OldPath: fields[i+1], Path: fields[i+2]})
			i += 3
			continue
		}
		if i+1 >= len(fields) {
			break
		}
		changes = append(changes, FileChange{Status: letter, Path: fields[i+1]})
		i += 2
	}
	return changes, nil
}

// StagedFileDiffs returns the staged patch split into one entry per path, so
// the budgeter can drop individual files without re-running git.
func StagedFileDiffs(ctx context.Context, dir string) ([]FileDiff, error) {
	changes, err := StagedNameStatus(ctx, dir)
	if err != nil {
		return nil, err
	}

	diffs := make([]FileDiff, 0, len(changes))
	for _, c := range changes {
		patch, err := run(ctx, dir, "diff", "--staged", "-M", "--", c.Path)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(patch) == "" {
			continue
		}
		diffs = append(diffs, FileDiff{Path: c.Path, Patch: patch})
	}
	return diffs, nil
}

// Commit writes message through `git commit -F -`, so no shell quoting or
// argument-length limit applies to the body.
func Commit(ctx context.Context, dir, message string, signOff bool) error {
	args := []string{"commit", "-F", "-"}
	if signOff {
		args = append(args, "--signoff")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(message)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return exitcode.Wrapf(exitcode.Generic, "git commit: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}
