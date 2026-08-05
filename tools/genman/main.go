// Command genman renders man/gitia.1 from the cobra command tree wrapped in
// man/gitia.1.tmpl. Run it with: make man
package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/guerrero/gitia/internal/cli"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "genman:", err)
		os.Exit(1)
	}
}

func run() error {
	root := cli.NewRootCmd()
	root.DisableAutoGenTag = true

	tmpl, err := template.ParseFiles("man/gitia.1.tmpl")
	if err != nil {
		return err
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, map[string]string{
		"Version":  cli.Version,
		"Date":     buildDate().Format("2006-01-02"),
		"Commands": renderCommands(root),
	}); err != nil {
		return err
	}

	return os.WriteFile("man/gitia.1", out.Bytes(), 0o644)
}

// buildDate honors SOURCE_DATE_EPOCH so CI can regenerate the page and compare
// it byte for byte against the committed one.
func buildDate() time.Time {
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		if secs, err := strconv.ParseInt(epoch, 10, 64); err == nil {
			return time.Unix(secs, 0).UTC()
		}
	}
	return time.Now().UTC()
}

// renderCommands emits a COMMANDS section plus each subcommand's flags, in
// roff, from the live command tree. cobra's own doc.GenMan is not used: it
// emits NAME and SYNOPSIS too, which the wrapper template already supplies.
func renderCommands(root *cobra.Command) string {
	var b strings.Builder

	b.WriteString(".SH COMMANDS\n")
	for _, c := range root.Commands() {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		fmt.Fprintf(&b, ".TP\n.B %s\n%s\n", c.Name(), c.Short)
	}

	b.WriteString(".SH OPTIONS\n")
	root.Flags().VisitAll(func(f *pflag.Flag) {
		writeFlag(&b, f)
	})

	for _, c := range root.Commands() {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		if !c.Flags().HasAvailableFlags() {
			continue
		}
		fmt.Fprintf(&b, ".SH OPTIONS FOR %s\n", strings.ToUpper(c.Name()))
		c.Flags().VisitAll(func(f *pflag.Flag) {
			writeFlag(&b, f)
		})
	}

	return b.String()
}

func writeFlag(b *strings.Builder, f *pflag.Flag) {
	b.WriteString(".TP\n")
	if f.Shorthand != "" {
		fmt.Fprintf(b, ".BR \\-%s \", \" \\-\\-%s\n", f.Shorthand, f.Name)
	} else {
		fmt.Fprintf(b, ".B \\-\\-%s\n", f.Name)
	}
	fmt.Fprintf(b, "%s\n", f.Usage)
}
