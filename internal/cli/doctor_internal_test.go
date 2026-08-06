package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintReportUsesUppercaseLabelsWithoutColor(t *testing.T) {
	r := Report{Checks: []Check{
		{Name: "git", Status: StatusPass, Detail: "git version 2.54.0"},
		{Name: "node", Status: StatusWarn, Detail: "node missing"},
		{Name: "model", Status: StatusFail, Detail: "not present"},
	}}

	var buf bytes.Buffer
	printReport(&buf, r)

	out := buf.String()
	for _, want := range []string{"OK", "WARN", "FAIL", "git", "node", "model"} {
		if !strings.Contains(out, want) {
			t.Errorf("printReport() is missing %q\n%s", want, out)
		}
	}
	// A bytes.Buffer is not a terminal: color codes must never leak into
	// piped or captured output.
	if strings.Contains(out, "\x1b[") {
		t.Errorf("printReport() emitted ANSI codes into a buffer:\n%q", out)
	}
}

func TestPrintReportAlignsTheStatusColumn(t *testing.T) {
	r := Report{Checks: []Check{
		{Name: "a", Status: StatusPass, Detail: "x"},
		{Name: "b", Status: StatusFail, Detail: "y"},
	}}

	var buf bytes.Buffer
	printReport(&buf, r)

	// The status column is fixed width, so the check names line up.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "OK  ") && !strings.HasPrefix(line, "WARN") && !strings.HasPrefix(line, "FAIL") {
			t.Errorf("line %q does not start with a padded status label", line)
		}
	}
}
