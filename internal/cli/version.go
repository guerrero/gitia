package cli

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// Populated at build time via -ldflags -X github.com/guerrero/gitia/internal/cli.Version=...
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// VersionString renders the multi-line version block, newline-terminated.
func VersionString() string {
	var b strings.Builder
	fmt.Fprintf(&b, "gitia %s\n", Version)
	fmt.Fprintf(&b, "commit:  %s\n", Commit)
	fmt.Fprintf(&b, "built:   %s\n", Date)
	fmt.Fprintf(&b, "go:      %s\n", runtime.Version())
	fmt.Fprintf(&b, "platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return b.String()
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Print(VersionString())
			return nil
		},
	}
}
