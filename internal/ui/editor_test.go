package ui_test

import (
	"strings"
	"testing"

	"github.com/guerrero/gitia/internal/exitcode"
	"github.com/guerrero/gitia/internal/ui"
)

func TestEditorPrecedence(t *testing.T) {
	t.Setenv("EDITOR", "")
	if got := ui.Editor(""); len(got) != 1 || got[0] != "vi" {
		t.Errorf("Editor() = %v, want [vi] when neither config nor EDITOR is set", got)
	}

	t.Setenv("EDITOR", "nano")
	if got := ui.Editor(""); got[0] != "nano" {
		t.Errorf("Editor() = %v, want EDITOR when config is unset", got)
	}

	t.Setenv("EDITOR", "vim")
	if got := ui.Editor("code --wait"); got[0] != "code" {
		t.Errorf("Editor() = %v, want the configured editor to win over EDITOR", got)
	}
}

func TestEditorConfigWinsOverEnvironment(t *testing.T) {
	t.Setenv("EDITOR", "vim")

	if got := ui.Editor("code --wait"); len(got) != 2 || got[0] != "code" {
		t.Errorf("Editor(config) = %v, want the configured editor to win", got)
	}

	if got := ui.Editor("  "); got[0] != "vim" {
		t.Errorf("Editor(blank config) = %v, want a blank configured value to fall through to the environment", got)
	}
}

func TestEditorSplitsArguments(t *testing.T) {
	t.Setenv("EDITOR", "code --wait")

	got := ui.Editor("")
	if len(got) != 2 || got[0] != "code" || got[1] != "--wait" {
		t.Errorf("Editor() = %v, want [code --wait]", got)
	}
}

func TestEditMessageRoundTrips(t *testing.T) {
	// An "editor" that appends a line, exercising the temp-file round trip.
	t.Setenv("EDITOR", "sh -c 'printf \"\\nedited\\n\" >> \"$1\"' --")

	got, err := ui.EditMessage(t.Context(), "feat: original\n", "")
	if err != nil {
		t.Fatalf("EditMessage: %v", err)
	}
	if !strings.Contains(got, "feat: original") {
		t.Errorf("EditMessage() = %q, want the original text preserved", got)
	}
	if !strings.Contains(got, "edited") {
		t.Errorf("EditMessage() = %q, want the editor's addition", got)
	}
}

func TestEditMessageStripsCommentLines(t *testing.T) {
	t.Setenv("EDITOR", "sh -c 'printf \"# a comment\\nkept\\n\" > \"$1\"' --")

	got, err := ui.EditMessage(t.Context(), "feat: original\n", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "a comment") {
		t.Errorf("EditMessage() = %q, want # lines stripped", got)
	}
	if !strings.Contains(got, "kept") {
		t.Errorf("EditMessage() = %q, want the non-comment line kept", got)
	}
}

func TestEditMessageEmptyBufferAborts(t *testing.T) {
	t.Setenv("EDITOR", "sh -c ': > \"$1\"' --")

	_, err := ui.EditMessage(t.Context(), "feat: original\n", "")
	if err == nil {
		t.Fatal("EditMessage() = nil error for an empty buffer, want an abort")
	}
	if got := exitcode.Of(err); got != exitcode.Aborted {
		t.Errorf("exit code = %d, want %d (Aborted)", got, exitcode.Aborted)
	}
}

func TestEditMessageEditorFailureIsAnError(t *testing.T) {
	t.Setenv("EDITOR", "false")

	if _, err := ui.EditMessage(t.Context(), "feat: original\n", ""); err == nil {
		t.Fatal("EditMessage() = nil error when the editor exited non-zero")
	}
}
