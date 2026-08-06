package ui_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/guerrero/gitia/internal/ui"
)

func TestSpinnerWritesNothingToANonTerminal(t *testing.T) {
	var out bytes.Buffer
	s := ui.StartSpinner(&out, "Processing...")
	s.Stop()

	if out.Len() != 0 {
		t.Errorf("StartSpinner wrote %q to a non-terminal, want nothing", out.String())
	}
}

func TestSpinnerStopIsIdempotent(t *testing.T) {
	var out bytes.Buffer
	s := ui.StartSpinner(&out, "Processing...")
	s.Stop()
	s.Stop()
}

func TestSpinnerStopsPromptly(t *testing.T) {
	var out bytes.Buffer
	s := ui.StartSpinner(&out, "Processing...")

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return promptly")
	}
}
