package ui

import (
	"fmt"
	"io"
	"time"
)

// spinnerFrames is the braille arc cycle; the frames advance fast enough to
// look like motion but slow enough to be legible over a slow model call.
var spinnerFrames = []string{"⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner animates a progress indicator on a real terminal. The animation runs
// in a background goroutine and is cleared on Stop; on a non-terminal writer it
// is a no-op so piped output stays clean.
type Spinner struct {
	stop   chan struct{}
	done   chan struct{}
	msg    string
	out    io.Writer
	active bool
}

// StartSpinner begins animating msg on out and returns a Spinner to Stop.
// Calling Stop more than once is safe.
func StartSpinner(out io.Writer, msg string) *Spinner {
	s := &Spinner{
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		msg:    msg,
		out:    out,
		active: IsTerminalWriter(out),
	}
	if !s.active {
		close(s.done)
		return s
	}

	go func() {
		defer close(s.done)
		for i := 0; ; i++ {
			select {
			case <-s.stop:
				return
			case <-time.After(100 * time.Millisecond):
			}
			fmt.Fprintf(s.out, "\r%s %s", spinnerFrames[i%len(spinnerFrames)], s.msg)
		}
	}()
	return s
}

// Stop halts the animation and erases the progress line. It never writes when
// the writer is not a terminal.
func (s *Spinner) Stop() {
	select {
	case <-s.stop:
		return
	default:
		close(s.stop)
		<-s.done
	}
	if s.active {
		fmt.Fprint(s.out, "\r\x1b[2K")
	}
}
