// Package exitcode defines gitia's process exit codes and the typed error
// that carries one. Only cmd/gitia/main.go turns these into an os.Exit call.
package exitcode

import (
	"errors"
	"fmt"
)

// Process exit codes. These are part of gitia's public contract; scripts and
// git hooks depend on them, so values must never be reassigned.
const (
	OK                 = 0
	Generic            = 1
	Usage              = 2
	NotRepo            = 3
	NothingStaged      = 4
	OllamaUnreachable  = 5
	ModelMissing       = 6
	GenerationFailed   = 7
	CommitlintRejected = 8
	Aborted            = 130
)

// Error is an error annotated with the exit code gitia should terminate with.
type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Wrap annotates err with code. It returns nil when err is nil so callers can
// write `return exitcode.Wrap(code, doThing())` unconditionally.
func Wrap(code int, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Err: err}
}

// Wrapf builds a new error with the given code and formatted message.
func Wrapf(code int, format string, a ...any) error {
	return &Error{Code: code, Err: fmt.Errorf(format, a...)}
}

// Of reports the exit code err asks for: OK for nil, the code of the outermost
// *Error in the chain, or Generic when the chain carries no code.
func Of(err error) int {
	if err == nil {
		return OK
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return Generic
}
