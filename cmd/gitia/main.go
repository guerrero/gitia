// Command gitia generates Conventional Commit messages from the staged diff
// using a local Ollama model.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/guerrero/gitia/internal/cli"
	"github.com/guerrero/gitia/internal/exitcode"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.NewRootCmd()
	root.SetArgs(cli.PreparseFixes(os.Args[1:]))

	// Cobra returns flag-parsing errors to the command's FlagErrorFunc; claim
	// them as usage errors (exit 2) before they reach exitcode.Of.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return exitcode.Wrap(exitcode.Usage, err)
	})

	if err := root.ExecuteContext(ctx); err != nil {
		if msg := err.Error(); msg != "" {
			fmt.Fprintln(os.Stderr, "gitia:", msg)
		}
		os.Exit(exitcode.Of(usageExit(err)))
	}
}

// usageExit marks cobra's plain usage errors with exitcode.Usage. Unknown
// commands and stray positional arguments are returned by cobra's find and
// args validation as untyped fmt.Errorf values, so they are recognized by
// their distinctive prefix. Flag errors never reach this function: the
// FlagErrorFunc above already wrapped them.
func usageExit(err error) error {
	if err != nil && strings.HasPrefix(err.Error(), "unknown command ") {
		return exitcode.Wrap(exitcode.Usage, err)
	}
	return err
}
