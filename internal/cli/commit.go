package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guerrero/gitia/internal/commit"
	"github.com/guerrero/gitia/internal/exitcode"
	"github.com/guerrero/gitia/internal/git"
	"github.com/guerrero/gitia/internal/ollama"
	"github.com/guerrero/gitia/internal/prompt"
	"github.com/guerrero/gitia/internal/rules"
	"github.com/guerrero/gitia/internal/ui"
)

type commitOptions struct {
	yes      bool
	model    string
	breaking bool
	fixes    []string
	dryRun   bool
	typ      string
	scope    string
	noBody   bool
	verbose  bool
}

func newCommitCmd() *cobra.Command {
	var o commitOptions

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Generate a commit message from the staged diff and commit it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommit(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), o)
		},
	}

	f := cmd.Flags()
	f.BoolVarP(&o.yes, "yes", "y", false, "skip the confirmation prompt")
	f.StringVarP(&o.model, "model", "m", "", "override the model for this run")
	f.BoolVarP(&o.breaking, "breaking", "b", false, "mark as a breaking change")
	f.StringSliceVarP(&o.fixes, "fixes", "f", nil, "issue numbers appended as Fixes #123, #456")
	f.BoolVarP(&o.dryRun, "dry-run", "n", false, "render and print, do not commit")
	f.StringVar(&o.typ, "type", "", "force the commit type")
	f.StringVar(&o.scope, "scope", "", "force the scope")
	f.BoolVar(&o.noBody, "no-body", false, "subject line only")
	// --verbose has no short form: -v is bound to version on the root command.
	f.BoolVar(&o.verbose, "verbose", false, "show resolved rules, prompt, and raw model output")

	return cmd
}

func runCommit(ctx context.Context, stdout, stderr io.Writer, o commitOptions) error {
	// The commit feedback marks are colored only on a real terminal; piped
	// output carries the plain symbols.
	check, cross := "✓", "✗"
	if ui.IsTerminalWriter(stdout) {
		check = ansiGreen + check + ansiReset
		cross = ansiRed + cross + ansiReset
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	// 1. Preflight: repository and staging area.
	repoRoot, err := git.RepoRoot(ctx, wd)
	if err != nil {
		return err
	}
	staged, err := git.HasStagedChanges(ctx, repoRoot)
	if err != nil {
		return err
	}
	if !staged {
		return exitcode.Wrapf(exitcode.NothingStaged,
			"no staged changes; stage them with git add")
	}

	cfgPath, err := rules.ConfigPath()
	if err != nil {
		return err
	}
	cfg, err := rules.LoadConfig(cfgPath)
	if err != nil {
		return err
	}

	client := ollama.New(cfg.OllamaHost(), cfg.OllamaTimeout())

	// 2. Model resolution, including the pull prompt.
	model, err := resolveModel(ctx, stdout, stderr, client, cfg, o)
	if err != nil {
		return err
	}

	// 3. Collect.
	stat, err := git.StagedStat(ctx, repoRoot)
	if err != nil {
		return err
	}
	changes, err := git.StagedNameStatus(ctx, repoRoot)
	if err != nil {
		return err
	}
	diffs, err := git.StagedFileDiffs(ctx, repoRoot)
	if err != nil {
		return err
	}

	stagedPaths := make([]string, 0, len(changes))
	for _, c := range changes {
		stagedPaths = append(stagedPaths, c.Path)
	}

	// 4. Resolve every layer, then apply commitlint as a hard gate.
	docs, err := rules.DiscoverAgentDocs(repoRoot, stagedPaths, cfg.Agents)
	if err != nil {
		return err
	}

	runner := rules.DetectRunner(repoRoot, cfg.Commitlint)
	var constraints *rules.Constraints
	if !runner.IsZero() {
		cacheDir, dirErr := rules.ConfigDir()
		if dirErr == nil {
			if data, cErr := rules.CachedPrintConfig(ctx, repoRoot, runner, cacheDir); cErr == nil {
				if c, pErr := rules.ParsePrintConfig(data); pErr == nil {
					constraints = &c
				} else if o.verbose {
					fmt.Fprintf(stderr, "gitia: ignoring commitlint config: %v\n", pErr)
				}
			} else if o.verbose {
				fmt.Fprintf(stderr, "gitia: commitlint --print-config failed: %v\n", cErr)
			}
		}
	}

	resolved := rules.Resolve(rules.Layers{
		Config:    cfg,
		AgentDocs: docs,
		Flags: rules.FlagOverrides{
			Type:     o.typ,
			Scope:    o.scope,
			NoBody:   o.noBody,
			Breaking: o.breaking,
			Model:    o.model,
		},
		Constraints: constraints,
	})

	for _, w := range resolved.Warnings() {
		fmt.Fprintln(stderr, w)
	}

	// 5. Build the prompt and schema.
	budgeted := prompt.Budget(
		prompt.DiffInput{Stat: stat, Changes: changes, Diffs: diffs},
		cfg.Diff.MaxBytes, cfg.Diff.Exclude)

	system := prompt.System(resolved.Rules, resolved.AgentDocs)
	user := prompt.User(budgeted)
	schema := prompt.Schema(resolved.Rules)

	if o.verbose {
		fmt.Fprintf(stderr, "--- resolved rules ---\n%+v\n", resolved.Rules)
		for _, n := range budgeted.Notes {
			fmt.Fprintf(stderr, "diff budget: %s\n", n)
		}
		fmt.Fprintf(stderr, "--- system prompt ---\n%s\n", system)
		fmt.Fprintf(stderr, "--- user prompt ---\n%s\n", user)
		fmt.Fprintf(stderr, "--- schema ---\n%s\n", schema)
	}

	// 6-10. Generate, validate, gate; the menu loop can re-roll.
	interactive := !o.yes && !o.dryRun && ui.IsTTY(os.Stdout)

	var previous *commit.Message
	for reroll := 0; ; reroll++ {
		// The spinner is a progress cue on a terminal; in --verbose the model's
		// raw output is interleaved with stderr, so an animated \r would corrupt it.
		var spin *ui.Spinner
		if !o.verbose {
			spin = ui.StartSpinner(stderr, "Processing...")
		}
		msg, err := generateOnce(ctx, stderr, client, generateArgs{
			model:       model,
			system:      system,
			user:        user,
			schema:      schema,
			cfg:         cfg,
			rs:          resolved.Rules,
			temperature: prompt.RerollTemperature(cfg.Model.Temperature, reroll),
			previous:    previous,
			verbose:     o.verbose,
		})
		if spin != nil {
			spin.Stop()
		}
		if err != nil {
			return err
		}

		// 8. Apply the flags the model was not asked about.
		if o.breaking {
			msg.Breaking = true
			if msg.BreakingDescription == "" {
				msg.BreakingDescription = msg.Subject
			}
		}
		if f := FormatFixes(o.fixes); f != "" {
			msg.Footers = append(msg.Footers, commit.Footer{Token: "Fixes", Value: f})
		}
		msg = commit.Repair(msg, resolved.Rules)

		rendered := commit.Render(msg, resolved.Rules)

		// 10. commitlint is the repository's own gate; run it last.
		if err := rules.CommitlintEdit(ctx, repoRoot, runner, rendered); err != nil {
			return err
		}

		// 12. Dry run.
		if o.dryRun {
			fmt.Fprint(stdout, rendered)
			return nil
		}

		// 13. Non-interactive.
		if !interactive {
			if err := git.Commit(ctx, repoRoot, rendered, resolved.Rules.SignOff); err != nil {
				return err
			}
			fmt.Fprintln(stdout, check, "committed")
			return nil
		}

		// 14. The menu.
		fmt.Fprintf(stdout, "\n%s\n", rendered)
		choice, err := ui.Confirm(ctx, os.Stdin, stdout)
		if err != nil {
			return err
		}

		switch choice {
		case ui.ChoiceCommit:
			if err := git.Commit(ctx, repoRoot, rendered, resolved.Rules.SignOff); err != nil {
				return err
			}
			fmt.Fprintln(stdout, check, "committed")
			return nil
		case ui.ChoiceEdit:
			edited, err := ui.EditMessage(ctx, rendered, cfg.Commit.Editor)
			if err != nil {
				return err
			}
			if err := rules.CommitlintEdit(ctx, repoRoot, runner, edited+"\n"); err != nil {
				return err
			}
			if err := git.Commit(ctx, repoRoot, edited+"\n", resolved.Rules.SignOff); err != nil {
				return err
			}
			fmt.Fprintln(stdout, check, "committed")
			return nil
		case ui.ChoiceRegenerate:
			cp := msg
			previous = &cp
			continue
		case ui.ChoiceQuit:
			fmt.Fprintln(stdout, cross, "Aborted")
			return exitcode.Wrapf(exitcode.Aborted, "")
		}
	}
}

type generateArgs struct {
	model       string
	system      string
	user        string
	schema      json.RawMessage
	cfg         rules.Config
	rs          rules.RuleSet
	temperature float64
	previous    *commit.Message
	verbose     bool
}

// generateOnce runs the model, then validates with one deterministic repair and
// at most one model retry that names the violations.
func generateOnce(ctx context.Context, stderr io.Writer, client *ollama.Client, a generateArgs) (commit.Message, error) {
	user := a.user
	if a.previous != nil {
		user = prompt.Reroll(user, *a.previous)
	}

	var msg commit.Message
	var violations []rules.Violation

	for attempt := 0; attempt < 2; attempt++ {
		resp, err := client.Generate(ctx, ollama.GenerateRequest{
			Model:     a.model,
			System:    a.system,
			Prompt:    user,
			Format:    a.schema,
			KeepAlive: a.cfg.KeepAlive(),
			Options: ollama.Options{
				Temperature: a.temperature,
				NumCtx:      a.cfg.Model.NumCtx,
			},
		})
		if err != nil {
			return commit.Message{}, err
		}
		if a.verbose {
			fmt.Fprintf(stderr, "--- raw model output ---\n%s\n", resp.Response)
		}

		msg, err = DecodeMessage(resp.Response)
		if err != nil {
			violations = []rules.Violation{{Rule: "json", Message: err.Error()}}
			user = prompt.Retry(user, violations)
			continue
		}

		msg = commit.Repair(msg, a.rs)
		violations = commit.Validate(msg, a.rs)
		if len(violations) == 0 {
			return msg, nil
		}
		user = prompt.Retry(user, violations)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "could not produce a valid message after one retry:\n")
	for _, v := range violations {
		fmt.Fprintf(&b, "  %s: %s\n", v.Rule, v.Message)
	}
	fmt.Fprintf(&b, "\nthe last attempt was:\n\n%s", commit.Render(msg, a.rs))
	return commit.Message{}, exitcode.Wrapf(exitcode.GenerationFailed, "%s", b.String())
}

// resolveModel picks the model to use, walking the fallback chain and offering
// to pull only when there is a terminal to ask on. gitia never installs
// anything implicitly.
func resolveModel(ctx context.Context, stdout, stderr io.Writer, client *ollama.Client, cfg rules.Config, o commitOptions) (string, error) {
	candidates := []string{}
	if o.model != "" {
		candidates = append(candidates, o.model)
	} else {
		candidates = append(candidates, cfg.Model.Name)
		candidates = append(candidates, cfg.Model.Fallback...)
	}

	for _, name := range candidates {
		has, err := client.Has(ctx, name)
		if err != nil {
			return "", err // exit 5: unreachable
		}
		if has {
			return name, nil
		}
	}

	want := candidates[0]
	if o.yes || !ui.IsTTY(os.Stdout) {
		return "", exitcode.Wrapf(exitcode.ModelMissing,
			"model %s is not available locally; run: ollama pull %s", want, want)
	}

	ok, err := ui.ConfirmYesNo(ctx, os.Stdin, stdout, fmt.Sprintf("pull %s?", want))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", exitcode.Wrapf(exitcode.ModelMissing,
			"model %s is not available locally; run: ollama pull %s", want, want)
	}

	var last string
	if err := client.Pull(ctx, want, func(p ollama.PullProgress) {
		line := p.Status
		if p.Total > 0 {
			line = fmt.Sprintf("%s %d%%", p.Status, 100*p.Completed/p.Total)
		}
		if line != last {
			fmt.Fprintf(stderr, "\r%-60s", line)
			last = line
		}
	}); err != nil {
		return "", err
	}
	fmt.Fprintln(stderr)

	return want, nil
}

// DecodeMessage parses the model's JSON into a Message. It tolerates a fenced
// code block and a body emitted as a single string rather than an array, both
// of which a model can still produce if the constrained decode is bypassed.
func DecodeMessage(raw string) (commit.Message, error) {
	raw = strings.TrimSpace(raw)
	if fenced := extractFence(raw); fenced != "" {
		raw = fenced
	}

	var wire struct {
		Type                string          `json:"type"`
		Scope               string          `json:"scope"`
		Subject             string          `json:"subject"`
		Body                json.RawMessage `json:"body"`
		Breaking            bool            `json:"breaking"`
		BreakingDescription string          `json:"breaking_description"`
	}
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		return commit.Message{}, fmt.Errorf("model did not return JSON: %w", err)
	}

	msg := commit.Message{
		Type:                wire.Type,
		Scope:               wire.Scope,
		Subject:             wire.Subject,
		Breaking:            wire.Breaking,
		BreakingDescription: wire.BreakingDescription,
	}

	if len(wire.Body) > 0 {
		var paragraphs []string
		if err := json.Unmarshal(wire.Body, &paragraphs); err == nil {
			msg.Body = paragraphs
		} else {
			var one string
			if err := json.Unmarshal(wire.Body, &one); err == nil && one != "" {
				msg.Body = []string{one}
			}
		}
	}

	if msg.Type == "" && msg.Subject == "" {
		return commit.Message{}, errors.New("model returned an empty message")
	}
	return msg, nil
}

// extractFence pulls the contents out of a ```json fence, returning "" when
// there is none.
func extractFence(s string) string {
	start := strings.Index(s, "```")
	if start < 0 {
		return ""
	}
	rest := s[start+3:]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:]
	}
	if end := strings.Index(rest, "```"); end >= 0 {
		return strings.TrimSpace(rest[:end])
	}
	return ""
}

// FormatFixes renders issue numbers as a Fixes footer value.
func FormatFixes(fixes []string) string {
	out := make([]string, 0, len(fixes))
	for _, f := range fixes {
		f = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(f), "#"))
		if f == "" {
			continue
		}
		out = append(out, "#"+f)
	}
	return strings.Join(out, ", ")
}
