package exitcode_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/guerrero/gitia/internal/exitcode"
)

func TestOf(t *testing.T) {
	sentinel := errors.New("boom")

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil is success", nil, exitcode.OK},
		{"unwrapped error is generic", sentinel, exitcode.Generic},
		{"wrapped error carries its code", exitcode.Wrap(exitcode.NothingStaged, sentinel), exitcode.NothingStaged},
		{"code survives fmt.Errorf wrapping", fmt.Errorf("ctx: %w", exitcode.Wrap(exitcode.NotRepo, sentinel)), exitcode.NotRepo},
		{"outermost code wins", exitcode.Wrap(exitcode.Aborted, exitcode.Wrap(exitcode.NotRepo, sentinel)), exitcode.Aborted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitcode.Of(tt.err); got != tt.want {
				t.Errorf("Of() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWrapNilStaysNil(t *testing.T) {
	if err := exitcode.Wrap(exitcode.Generic, nil); err != nil {
		t.Errorf("Wrap(code, nil) = %v, want nil", err)
	}
}

func TestUnwrapReachesCause(t *testing.T) {
	sentinel := errors.New("boom")
	if !errors.Is(exitcode.Wrap(exitcode.Usage, sentinel), sentinel) {
		t.Error("errors.Is could not reach the wrapped cause")
	}
}
