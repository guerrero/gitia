package cli_test

import (
	"strings"
	"testing"

	"github.com/guerrero/gitia/internal/cli"
)

func TestDecodeMessage(t *testing.T) {
	raw := `{
		"type": "feat",
		"scope": "commit",
		"subject": "generate messages from the staged diff",
		"body": ["First paragraph.", "Second paragraph."],
		"breaking": false
	}`

	got, err := cli.DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if got.Type != "feat" {
		t.Errorf("Type = %q, want feat", got.Type)
	}
	if got.Scope != "commit" {
		t.Errorf("Scope = %q, want commit", got.Scope)
	}
	if len(got.Body) != 2 || got.Body[1] != "Second paragraph." {
		t.Errorf("Body = %q, want two paragraphs", got.Body)
	}
}

func TestDecodeMessageBreaking(t *testing.T) {
	raw := `{"type":"feat","subject":"x","breaking":true,"breaking_description":"the api changed"}`

	got, err := cli.DecodeMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Breaking {
		t.Error("Breaking = false, want true")
	}
	if got.BreakingDescription != "the api changed" {
		t.Errorf("BreakingDescription = %q, want %q", got.BreakingDescription, "the api changed")
	}
}

func TestDecodeMessageToleratesFencedJSON(t *testing.T) {
	raw := "```json\n{\"type\":\"fix\",\"subject\":\"x\"}\n```"

	got, err := cli.DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if got.Type != "fix" {
		t.Errorf("Type = %q, want fix", got.Type)
	}
}

func TestDecodeMessageToleratesAStringBody(t *testing.T) {
	// The schema asks for an array, but a model can still emit a string when
	// the constrained decode is bypassed by an older Ollama.
	raw := `{"type":"fix","subject":"x","body":"one paragraph"}`

	got, err := cli.DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if len(got.Body) != 1 || got.Body[0] != "one paragraph" {
		t.Errorf("Body = %q, want a single-element slice", got.Body)
	}
}

func TestDecodeMessageRejectsGarbage(t *testing.T) {
	if _, err := cli.DecodeMessage("I think this change adds a feature."); err == nil {
		t.Fatal("DecodeMessage() = nil error for prose, want an error")
	}
}

func TestFormatFixes(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"123"}, "#123"},
		{[]string{"123", "456"}, "#123, #456"},
		{[]string{"#123", "456"}, "#123, #456"},
		{[]string{"123", "", "456"}, "#123, #456"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.in, "_"), func(t *testing.T) {
			if got := cli.FormatFixes(tt.in); got != tt.want {
				t.Errorf("FormatFixes(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
