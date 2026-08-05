package ui_test

import (
	"bytes"
	"strings"
	"testing"

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
			got, err := ui.Prompt(strings.NewReader(tt.input), &out)
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
	got, err := ui.Prompt(strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != ui.ChoiceQuit {
		t.Errorf("Prompt(EOF) = %q, want quit", got)
	}
}

func TestPromptShowsEveryOption(t *testing.T) {
	var out bytes.Buffer
	if _, err := ui.Prompt(strings.NewReader("y"), &out); err != nil {
		t.Fatal(err)
	}

	rendered := out.String()
	for _, want := range []string{"[y]", "[e]", "[r]", "[q]", "commit", "regenerate", "abort"} {
		if !strings.Contains(strings.ToLower(rendered), strings.ToLower(want)) {
			t.Errorf("prompt %q is missing %q", rendered, want)
		}
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
			got, err := ui.ConfirmYesNo(strings.NewReader(tt.input), &out, "pull gemma4:e2b-it-qat (~4.3GB)?")
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
	if _, err := ui.ConfirmYesNo(strings.NewReader("\n"), &out, "pull it?"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[y/N]") {
		t.Errorf("prompt = %q, want it to show [y/N]", out.String())
	}
}
