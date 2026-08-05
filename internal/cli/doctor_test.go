package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/guerrero/gitia/internal/cli"
)

func tempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func checkByName(t *testing.T, r cli.Report, name string) cli.Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("report has no check named %q: %+v", name, r.Checks)
	return cli.Check{}
}

func TestRunChecksReturnsTenChecksInOrder(t *testing.T) {
	// Point the Ollama host somewhere closed so the run is fast and offline.
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
	t.Setenv("GITIA_CONFIG_DIR", t.TempDir())

	got := cli.RunChecks(t.Context(), tempRepo(t))

	if len(got.Checks) != 10 {
		t.Fatalf("RunChecks() returned %d checks, want 10: %+v", len(got.Checks), got.Checks)
	}

	wantOrder := []string{
		"git", "repository", "ollama binary", "ollama server",
		"model", "node", "commitlint", "config", "editor", "conventions",
	}
	for i, want := range wantOrder {
		if got.Checks[i].Name != want {
			t.Errorf("Checks[%d].Name = %q, want %q", i, got.Checks[i].Name, want)
		}
	}
}

func TestRunChecksInsideARepository(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
	t.Setenv("GITIA_CONFIG_DIR", t.TempDir())

	got := cli.RunChecks(t.Context(), tempRepo(t))

	if c := checkByName(t, got, "git"); c.Status != cli.StatusPass {
		t.Errorf("git check = %+v, want pass", c)
	}
	if c := checkByName(t, got, "repository"); c.Status != cli.StatusPass {
		t.Errorf("repository check = %+v, want pass", c)
	}
}

func TestRunChecksOutsideARepositoryFails(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
	t.Setenv("GITIA_CONFIG_DIR", t.TempDir())

	got := cli.RunChecks(t.Context(), t.TempDir())

	if c := checkByName(t, got, "repository"); c.Status != cli.StatusFail {
		t.Errorf("repository check = %+v, want fail outside a work tree", c)
	}
	if got.OK {
		t.Error("OK = true with a failing check")
	}
}

func TestRunChecksUnreachableOllamaFails(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
	t.Setenv("GITIA_CONFIG_DIR", t.TempDir())

	got := cli.RunChecks(t.Context(), tempRepo(t))

	if c := checkByName(t, got, "ollama server"); c.Status != cli.StatusFail {
		t.Errorf("ollama server check = %+v, want fail", c)
	}
}

func TestChecksSixThroughTenOnlyWarn(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
	t.Setenv("GITIA_CONFIG_DIR", t.TempDir())
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	got := cli.RunChecks(t.Context(), tempRepo(t))

	// A bare repository with no Node, no commitlint, no config, no editor, and
	// no AGENTS.md must produce warnings, never failures, for checks 6-10.
	for _, name := range []string{"node", "commitlint", "config", "editor", "conventions"} {
		if c := checkByName(t, got, name); c.Status == cli.StatusFail {
			t.Errorf("%s check = %+v; checks 6-10 must never fail", name, c)
		}
	}
}

func TestChecksDetectConventionFiles(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
	t.Setenv("GITIA_CONFIG_DIR", t.TempDir())

	dir := tempRepo(t)
	if err := os.WriteFile(dir+"/AGENTS.md", []byte("## Commit\n\nrule\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := cli.RunChecks(t.Context(), dir)
	if c := checkByName(t, got, "conventions"); c.Status != cli.StatusPass {
		t.Errorf("conventions check = %+v, want pass with an AGENTS.md present", c)
	}
}

func TestReportMarshalsToStableJSON(t *testing.T) {
	r := cli.Report{
		OK: true,
		Checks: []cli.Check{{
			Name: "git", Status: cli.StatusPass, Detail: "git version 2.51.0",
		}},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}

	var back struct {
		OK     bool `json:"ok"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Report JSON is not the documented shape: %v\n%s", err, data)
	}
	if !back.OK || len(back.Checks) != 1 || back.Checks[0].Status != "pass" {
		t.Errorf("round-tripped report = %+v", back)
	}
}
