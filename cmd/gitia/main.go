// Command gitia generates Conventional Commit messages from the staged diff
// using a local Ollama model.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/guerrero/gitia/internal/cli"
	"github.com/guerrero/gitia/internal/exitcode"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.NewRootCmd()
	root.SetArgs(cli.PreparseFixes(os.Args[1:]))

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "gitia:", err)
		os.Exit(exitcode.Of(err))
	}
}
