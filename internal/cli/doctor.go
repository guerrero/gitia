package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/guerrero/gitia/internal/exitcode"
	"github.com/guerrero/gitia/internal/git"
	"github.com/guerrero/gitia/internal/ollama"
	"github.com/guerrero/gitia/internal/rules"
	"github.com/guerrero/gitia/internal/ui"
)

// CheckStatus is the outcome of one doctor check.
type CheckStatus string

// Status values reported by doctor checks.
const (
	StatusPass CheckStatus = "pass"
	StatusWarn CheckStatus = "warn"
	StatusFail CheckStatus = "fail"
)

// Check is one diagnostic line.
type Check struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
	Detail string      `json:"detail"`
}

// Report is the full doctor result. OK is false when any check failed;
// warnings never affect it.
type Report struct {
	Checks []Check `json:"checks"`
	OK     bool    `json:"ok"`
}

func newDoctorCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose gitia's environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			report := RunChecks(cmd.Context(), wd)

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(report); err != nil {
					return err
				}
			} else {
				printReport(cmd.OutOrStdout(), report)
			}

			if !report.OK {
				return exitcode.Wrapf(exitcode.Generic, "one or more checks failed")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable results")
	return cmd
}

// printReport renders the checks one per line. On a real terminal the status is
// colored green/yellow/red; the JSON form is the machine-readable source of
// truth and carries no color.
func printReport(w io.Writer, r Report) {
	labels := map[CheckStatus]string{
		StatusPass: "OK",
		StatusWarn: "WARN",
		StatusFail: "FAIL",
	}
	colors := map[CheckStatus]string{
		StatusPass: ansiGreen,
		StatusWarn: ansiYellow,
		StatusFail: ansiRed,
	}
	colored := ui.IsTerminalWriter(w)
	for _, c := range r.Checks {
		label := labels[c.Status]
		if !colored {
			fmt.Fprintf(w, "%-4s  %-16s %s\n", label, c.Name, c.Detail)
			continue
		}
		fmt.Fprintf(w, "%s%-4s%s  %-16s %s\n", colors[c.Status], label, ansiReset, c.Name, c.Detail)
	}
}

// ANSI SGR codes for the doctor status labels.
const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
)

// RunChecks executes the ten diagnostics in order. Checks 6-10 can only pass or
// warn: gitia works without Node, commitlint, a config file, an editor, or any
// convention files.
func RunChecks(ctx context.Context, dir string) Report {
	var r Report
	add := func(name string, status CheckStatus, format string, a ...any) {
		r.Checks = append(r.Checks, Check{name, status, fmt.Sprintf(format, a...)})
	}

	// 1. git on PATH.
	if v, err := git.Version(ctx); err != nil {
		add("git", StatusFail, "git not found on PATH")
	} else {
		add("git", StatusPass, "%s", v)
	}

	// 2. Inside a work tree.
	repoRoot, repoErr := git.RepoRoot(ctx, dir)
	if repoErr != nil {
		add("repository", StatusFail, "not inside a git work tree")
	} else {
		add("repository", StatusPass, "%s", repoRoot)
	}

	// 3. The ollama binary. Its absence is only a warning path away from
	// fatal: gitia talks HTTP and does not need the CLI, but without it the
	// user has no way to start a server.
	if path, err := exec.LookPath("ollama"); err != nil {
		add("ollama binary", StatusFail, "ollama not found on PATH; install it with: brew install ollama")
	} else {
		add("ollama binary", StatusPass, "%s", path)
	}

	cfgPath, pathErr := rules.ConfigPath()
	cfg := rules.DefaultConfig()
	var cfgFound bool
	var cfgErr error
	if pathErr == nil {
		cfg, cfgFound, cfgErr = rules.LoadConfigAt(cfgPath)
	}

	// 4. Server reachable.
	client := ollama.New(cfg.OllamaHost(), cfg.OllamaTimeout())
	models, tagsErr := client.Tags(ctx)
	if tagsErr != nil {
		add("ollama server", StatusFail, "%s unreachable; start it with: ollama serve", client.Host())
	} else {
		add("ollama server", StatusPass, "%s (%d models)", client.Host(), len(models))
	}

	// 5. The default model.
	switch {
	case tagsErr != nil:
		add("model", StatusFail, "cannot check %s: server unreachable", cfg.Model.Name)
	default:
		var found *ollama.Model
		if m, err := client.Show(ctx, cfg.Model.Name); err == nil {
			found = m
		}
		if found != nil {
			add("model", StatusPass, "%s (%.1f GB)", cfg.Model.Name, float64(found.Size)/1e9)
		} else {
			add("model", StatusFail, "%s not present; run: ollama pull %s", cfg.Model.Name, cfg.Model.Name)
		}
	}

	// 6. Node and a package manager. Warning only.
	if nodePath, err := exec.LookPath("node"); err != nil {
		add("node", StatusWarn, "node not found; commitlint integration is unavailable")
	} else {
		runner := rules.NoRunner
		if repoErr == nil {
			runner = rules.DetectRunner(repoRoot, cfg.Commitlint)
		}
		if runner.IsZero() {
			add("node", StatusWarn, "%s found, but no package manager lockfile in this repository", nodePath)
		} else {
			add("node", StatusPass, "%s, runner: %s", nodePath, runner.Name)
		}
	}

	// 7. commitlint config resolves. Warning only.
	switch {
	case repoErr != nil:
		add("commitlint", StatusWarn, "not checked: not inside a repository")
	default:
		configPath, hasConfig := rules.HasCommitlintConfig(repoRoot)
		runner := rules.DetectRunner(repoRoot, cfg.Commitlint)
		switch {
		case !hasConfig:
			add("commitlint", StatusWarn, "no commitlint config in this repository")
		case runner.IsZero():
			add("commitlint", StatusWarn, "%s found, but no runner detected", filepath.Base(configPath))
		default:
			if _, err := rules.PrintConfig(ctx, repoRoot, runner); err != nil {
				add("commitlint", StatusWarn, "%s --print-config json failed: %v", runner.Name, err)
			} else {
				add("commitlint", StatusPass, "%s resolves via %s", filepath.Base(configPath), runner.Name)
			}
		}
	}

	// 8. gitia's own config. Warning only: every value has a default.
	switch {
	case pathErr != nil:
		add("config", StatusWarn, "cannot locate a config directory: %v", pathErr)
	case cfgErr != nil:
		add("config", StatusWarn, "%s does not parse: %v", cfgPath, cfgErr)
	case !cfgFound:
		add("config", StatusWarn, "%s not present; using defaults", cfgPath)
	default:
		add("config", StatusPass, "%s", cfgPath)
	}

	// 9. An editor. Warning only: [e] is optional, and the resolution always
	// falls back to vi.
	switch {
	case cfg.Commit.Editor != "":
		add("editor", StatusPass, "editor=%s", cfg.Commit.Editor)
	case os.Getenv("EDITOR") != "":
		add("editor", StatusPass, "EDITOR=%s", os.Getenv("EDITOR"))
	default:
		add("editor", StatusWarn, "EDITOR is not set; [e] will use vi")
	}

	// 10. Convention files. Warning only.
	switch {
	case repoErr != nil:
		add("conventions", StatusWarn, "not checked: not inside a repository")
	default:
		rel, relErr := filepath.Rel(repoRoot, dir)
		if relErr != nil {
			rel = "."
		}
		docs, err := rules.DiscoverAgentDocs(repoRoot, []string{filepath.Join(rel, "x")}, cfg.Agents)
		switch {
		case err != nil:
			add("conventions", StatusWarn, "discovery failed: %v", err)
		case len(docs) == 0:
			add("conventions", StatusWarn, "no AGENTS.md, CLAUDE.md, or GEMINI.md found")
		default:
			names := make([]string, 0, len(docs))
			for _, d := range docs {
				names = append(names, d.Path)
			}
			add("conventions", StatusPass, "%v", names)
		}
	}

	r.OK = true
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			r.OK = false
		}
	}
	return r
}
