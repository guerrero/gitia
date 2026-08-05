package man_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestManPageIsCommitted guards against the committed page drifting from the
// command tree. It regenerates into a scratch copy and compares the parts that
// do not depend on the build date.
func TestManPageIsCommitted(t *testing.T) {
	committed, err := os.ReadFile("gitia.1")
	if err != nil {
		t.Fatalf("man/gitia.1 is not committed: %v (run: make man)", err)
	}

	cmd := exec.Command("go", "run", "./tools/genman")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go run ./tools/genman: %v\n%s", err, out)
	}

	regenerated, err := os.ReadFile("gitia.1")
	if err != nil {
		t.Fatal(err)
	}

	if stripDate(string(committed)) != stripDate(string(regenerated)) {
		// Restore what was committed so a failing test leaves no diff behind.
		_ = os.WriteFile("gitia.1", committed, 0o644)
		t.Error("man/gitia.1 is stale; run: make man")
	}
}

// stripDate drops the .TH line, which carries the generation date.
func stripDate(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, ".TH ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func TestManPageHasTheSectionsCobraDoesNotEmit(t *testing.T) {
	data, err := os.ReadFile("gitia.1")
	if err != nil {
		t.Fatal(err)
	}

	for _, section := range []string{
		".SH NAME", ".SH SYNOPSIS", ".SH DESCRIPTION", ".SH COMMANDS",
		".SH OPTIONS", ".SH ENVIRONMENT", ".SH FILES", ".SH EXIT STATUS", ".SH SEE ALSO",
	} {
		if !strings.Contains(string(data), section) {
			t.Errorf("man/gitia.1 is missing %s", section)
		}
	}
}

func TestManPageDocumentsEveryExitCode(t *testing.T) {
	data, err := os.ReadFile("gitia.1")
	if err != nil {
		t.Fatal(err)
	}

	for _, code := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "130"} {
		if !strings.Contains(string(data), ".B "+code+"\n") {
			t.Errorf("man/gitia.1 does not document exit code %s", code)
		}
	}
}
