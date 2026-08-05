package ui_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guerrero/gitia/internal/exitcode"
	"github.com/guerrero/gitia/internal/ui"
)

func TestPrompt(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  ui.Choice
	}{
		{"commit", "y", ui.ChoiceCommit},
		{"commit uppercase", "Y", ui.ChoiceCommit},
		{"enter defaults to commit", "\n", ui.ChoiceCommit},
		{"edit", "e", ui.ChoiceEdit},
		{"regenerate", "r", ui.ChoiceRegenerate},
		{"quit", "q", ui.ChoiceQuit},
		{"quit uppercase", "Q", ui.ChoiceQuit},
		{"garbage then a valid choice", "zx9q", ui.ChoiceQuit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := ui.Prompt(t.Context(), strings.NewReader(tt.input), &out)
			if err != nil {
				t.Fatalf("Prompt: %v", err)
			}
			if got != tt.want {
				t.Errorf("Prompt(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPromptEOFQuits(t *testing.T) {
	var out bytes.Buffer
	got, err := ui.Prompt(t.Context(), strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != ui.ChoiceQuit {
		t.Errorf("Prompt(EOF) = %q, want quit", got)
	}
}

func TestPromptShowsEveryOption(t *testing.T) {
	var out bytes.Buffer
	if _, err := ui.Prompt(t.Context(), strings.NewReader("y"), &out); err != nil {
		t.Fatal(err)
	}

	rendered := out.String()
	for _, want := range []string{"[y]", "[e]", "[r]", "[q]", "commit", "regenerate", "abort"} {
		if !strings.Contains(strings.ToLower(rendered), strings.ToLower(want)) {
			t.Errorf("prompt %q is missing %q", rendered, want)
		}
	}
}

// countingReader counts the bytes its Read calls hand out, so a test can tell
// whether a blocked prompt consumed anything.
type countingReader struct {
	r  io.Reader
	rd atomic.Int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.rd.Add(int64(n))
	return n, err
}

func TestPromptCancellationAbortsABlockedRead(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out bytes.Buffer
	in := &countingReader{r: pr}
	done := make(chan struct{})
	var got ui.Choice
	var err error
	go func() {
		got, err = ui.Prompt(ctx, in, &out)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Prompt did not return after the context was canceled")
	}
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != ui.ChoiceQuit {
		t.Errorf("Prompt = %q, want quit", got)
	}
	if n := in.rd.Load(); n != 0 {
		t.Errorf("Prompt consumed %d bytes; the abort must not need a keypress", n)
	}
}

func TestConfirmCancellationAbortsABlockedRead(t *testing.T) {
	pr, pw, perr := os.Pipe()
	if perr != nil {
		t.Fatal(perr)
	}
	defer pw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out bytes.Buffer
	done := make(chan struct{})
	var got ui.Choice
	var err error
	go func() {
		got, err = ui.Confirm(ctx, pr, &out)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Confirm did not return after the context was canceled")
	}
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if got != ui.ChoiceQuit {
		t.Errorf("Confirm = %q, want quit", got)
	}
}

func TestConfirmYesNoDefaultsToNo(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"n\n", false},
		{"\n", false},
		{"", false},
		{"garbage\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var out bytes.Buffer
			got, err := ui.ConfirmYesNo(t.Context(), strings.NewReader(tt.input), &out, "pull gemma4:e2b-it-qat (~4.3GB)?")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("ConfirmYesNo(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestConfirmYesNoShowsTheDefault(t *testing.T) {
	var out bytes.Buffer
	if _, err := ui.ConfirmYesNo(t.Context(), strings.NewReader("\n"), &out, "pull it?"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[y/N]") {
		t.Errorf("prompt = %q, want it to show [y/N]", out.String())
	}
}

func TestConfirmYesNoCancellationAbortsABlockedRead(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out bytes.Buffer
	done := make(chan struct{})
	var ok bool
	var err error
	go func() {
		ok, err = ui.ConfirmYesNo(ctx, pr, &out, "pull it?")
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ConfirmYesNo did not return after the context was canceled")
	}
	if err == nil {
		t.Fatal("ConfirmYesNo() = nil error; a canceled context must abort")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("ConfirmYesNo() = %v, want a context.Canceled cause", err)
	}
	if got := exitcode.Of(err); got != exitcode.Aborted {
		t.Errorf("exitcode.Of(err) = %d, want %d", got, exitcode.Aborted)
	}
	if ok {
		t.Error("ConfirmYesNo() = true, want false on cancellation")
	}
}
