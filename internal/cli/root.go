// Package cli defines gitia's cobra command tree.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewRootCmd builds the gitia command tree. It is a function rather than a
// package-level var so tests can build an isolated tree per case.
func NewRootCmd() *cobra.Command {
	var showVersion bool

	root := &cobra.Command{
		Use:   "gitia",
		Short: "Generate Conventional Commits from your staged diff with a local model",
		Long: "gitia generates Conventional Commit messages from the staged diff using a\n" +
			"model running locally under Ollama. Nothing leaves your machine.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				fmt.Fprint(cmd.OutOrStdout(), VersionString())
				return nil
			}
			return cmd.Help()
		},
	}

	// -v is bound to version rather than the more common verbose; as a result
	// `gitia commit --verbose` deliberately has no short form.
	root.Flags().BoolVarP(&showVersion, "version", "v", false, "print version information")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newCommitCmd())
	root.AddCommand(newDoctorCmd())

	enableCompletion(root)

	return root
}
