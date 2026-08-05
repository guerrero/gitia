package cli

import "github.com/spf13/cobra"

// enableCompletion configures cobra's built-in completion command. bash, zsh,
// and fish come at no cost; PowerShell is dropped because gitia does not
// support Windows.
func enableCompletion(root *cobra.Command) {
	root.CompletionOptions = cobra.CompletionOptions{
		DisableDefaultCmd:   false,
		DisableDescriptions: false,
	}
}
