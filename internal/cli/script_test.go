package cli_test

import (
	"os"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/spf13/cobra"

	"github.com/guerrero/gitia/internal/cli"
	"github.com/guerrero/gitia/internal/exitcode"
)

// TestMain registers gitia as a testscript command so the .txtar files can
// invoke the real command tree in-process, with real exit codes.
func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"gitia": func() int {
			root := cli.NewRootCmd()
			root.SetArgs(cli.PreparseFixes(os.Args[1:]))
			// Mirrors cmd/gitia/main.go, which the harness cannot import:
			// flag errors and cobra's unknown-command errors are usage errors.
			root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
				return exitcode.Wrap(exitcode.Usage, err)
			})
			if err := root.Execute(); err != nil {
				os.Stderr.WriteString("gitia: " + err.Error() + "\n")
				if strings.HasPrefix(err.Error(), "unknown command ") {
					err = exitcode.Wrap(exitcode.Usage, err)
				}
				return exitcode.Of(err)
			}
			return 0
		},
	}))
}

func TestScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(e *testscript.Env) error {
			// testscript sandboxes PATH and would hide git (and node, etc.) on
			// the system; prepend the real one so the scripts can run git.
			e.Setenv("PATH", os.Getenv("PATH"))
			// Keep every script offline and away from the developer's own
			// config: an unroutable host makes Ollama checks fail fast.
			e.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
			e.Setenv("GITIA_CONFIG_DIR", e.WorkDir+"/gitia-config")
			e.Setenv("EDITOR", "")
			e.Setenv("VISUAL", "")
			return nil
		},
		RequireExplicitExec: true,
	})
}
