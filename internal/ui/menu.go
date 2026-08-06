package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/guerrero/gitia/internal/exitcode"
)

// Choice is one entry from the confirmation menu.
type Choice rune

// Choices accepted by the confirmation prompt.
const (
	ChoiceCommit     Choice = 'y'
	ChoiceEdit       Choice = 'e'
	ChoiceRegenerate Choice = 'r'
	ChoiceQuit       Choice = 'q'
)

const menu = "[y] commit  [e] edit  [r] regenerate  [q] abort: "

// Prompt reads one choice from in, re-prompting on anything unrecognized.
// Enter alone means commit; EOF means quit. A canceled context means quit too:
// a blocked keypress must not survive the interrupt the context was canceled
// for, and the pipeline treats quit as the 130 abort.
func Prompt(ctx context.Context, in io.Reader, out io.Writer) (Choice, error) {
	reader := bufio.NewReader(in)

	for {
		if _, err := fmt.Fprint(out, menu); err != nil {
			return ChoiceQuit, err
		}

		r, err := ctxRead(ctx, func() (rune, error) {
			r, _, err := reader.ReadRune()
			return r, err
		})
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ChoiceQuit, nil
		}
		if errors.Is(err, io.EOF) {
			fmt.Fprintln(out)
			return ChoiceQuit, nil
		}
		if err != nil {
			return ChoiceQuit, err
		}

		switch r {
		case '\n', '\r', 'y', 'Y':
			fmt.Fprintln(out)
			return ChoiceCommit, nil
		case 'e', 'E':
			fmt.Fprintln(out)
			return ChoiceEdit, nil
		case 'r', 'R':
			fmt.Fprintln(out)
			return ChoiceRegenerate, nil
		case 'q', 'Q':
			fmt.Fprintln(out)
			return ChoiceQuit, nil
		}
		fmt.Fprintln(out)
	}
}

// Confirm shows the menu on a real terminal. It puts the terminal into raw
// mode so a single keypress is enough, and falls back to line-buffered input
// when raw mode is unavailable — the behavior is identical, the user just has
// to press Enter.
//
// Raw mode is not in the Go standard library, so it is set with stty. gitia
// targets macOS and Linux only, where stty is always present.
func Confirm(ctx context.Context, in *os.File, out io.Writer) (Choice, error) {
	restore, err := rawMode(in)
	if err == nil {
		defer restore()
	}
	return Prompt(ctx, in, out)
}

// rawMode puts the terminal into cbreak mode and returns a function that
// restores it. An error means the caller should fall back to line mode.
func rawMode(in *os.File) (func(), error) {
	if !IsTTY(in) {
		return nil, errors.New("not a terminal")
	}

	saved, err := stty(in, "-g")
	if err != nil {
		return nil, err
	}
	if _, err := stty(in, "-icanon", "-echo", "min", "1", "time", "0"); err != nil {
		return nil, err
	}

	return func() {
		// Restoring the terminal must not be skipped, but a failure here is
		// not worth failing the commit over.
		_, _ = stty(in, strings.Fields(saved)...)
	}, nil
}

func stty(in *os.File, args ...string) (string, error) {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = in

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ConfirmYesNo asks a yes/no question that defaults to no. gitia never
// installs anything implicitly, so the default must always be the inert one.
// A canceled context aborts: the error carries exitcode.Aborted so the caller
// does not need to distinguish a cancel from a "no".
func ConfirmYesNo(ctx context.Context, in io.Reader, out io.Writer, question string) (bool, error) {
	if _, err := fmt.Fprintf(out, "%s [y/N] ", question); err != nil {
		return false, err
	}

	line, err := ctxRead(ctx, func() (string, error) {
		return bufio.NewReader(in).ReadString('\n')
	})
	if err != nil && !errors.Is(err, io.EOF) {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, exitcode.Wrap(exitcode.Aborted, err)
		}
		return false, err
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// ctxRead runs the blocking read f and returns its result, or ctx.Err() when
// the context is canceled first. A blocked read cannot be interrupted, so the
// goroutine running it stays alive until the read returns — on a terminal that
// means process exit, and the result is discarded.
func ctxRead[T any](ctx context.Context, f func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := f()
		ch <- result{v, err}
	}()
	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case r := <-ch:
		return r.v, r.err
	}
}
