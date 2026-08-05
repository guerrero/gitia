package man_test

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestManPageIsCommitted guards against the committed page drifting from the
// command tree. It regenerates the page with SOURCE_DATE_EPOCH pinned to the
// date the committed page was generated under, so regeneration is
// deterministic and the comparison is byte for byte.
func TestManPageIsCommitted(t *testing.T) {
	committed, err := os.ReadFile("gitia.1")
	if err != nil {
		t.Fatalf("man/gitia.1 is not committed: %v (run: make man)", err)
	}

	cmd := exec.Command("go", "run", "./tools/genman")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "SOURCE_DATE_EPOCH="+committedEpoch(t, string(committed)))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go run ./tools/genman: %v\n%s", err, out)
	}

	regenerated, err := os.ReadFile("gitia.1")
	if err != nil {
		t.Fatal(err)
	}

	if string(committed) != string(regenerated) {
		t.Error("man/gitia.1 is stale; run: make man")
	}
}

// committedEpoch returns the SOURCE_DATE_EPOCH value that reproduces the date
// the committed page was generated under. The .TH line reads
// .TH GITIA 1 "2006-01-02" ..., and genman renders that date from the epoch it
// was given, so the epoch regenerates the same page byte for byte.
func committedEpoch(t *testing.T, page string) string {
	t.Helper()
	line, _, ok := strings.Cut(page, "\n")
	if !ok || !strings.HasPrefix(line, ".TH ") {
		t.Fatalf("man/gitia.1 does not start with a .TH line")
	}
	_, rest, ok := strings.Cut(line, `"`)
	if !ok {
		t.Fatalf(".TH line carries no date: %q", line)
	}
	date, _, ok := strings.Cut(rest, `"`)
	if !ok || date == "" {
		t.Fatalf(".TH line carries no date: %q", line)
	}
	when, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatalf("unparseable .TH date %q: %v", date, err)
	}
	return strconv.FormatInt(when.UTC().Unix(), 10)
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
