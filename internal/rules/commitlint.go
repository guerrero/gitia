package rules

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/guerrero/gitia/internal/exitcode"
)

// Runner is the command that invokes the repository's local commitlint.
type Runner struct {
	Name string
	Argv []string
}

// NoRunner means commitlint is unavailable or disabled.
var NoRunner = Runner{}

// IsZero reports whether r names no runnable commitlint.
func (r Runner) IsZero() bool { return len(r.Argv) == 0 }

// commitlintConfigNames are the config filenames commitlint itself looks for,
// checked at the repository root.
var commitlintConfigNames = []string{
	"commitlint.config.js", "commitlint.config.mjs", "commitlint.config.cjs",
	"commitlint.config.ts", "commitlint.config.mts", "commitlint.config.cts",
	".commitlintrc", ".commitlintrc.json", ".commitlintrc.yml", ".commitlintrc.yaml",
	".commitlintrc.js", ".commitlintrc.mjs", ".commitlintrc.cjs", ".commitlintrc.ts",
}

// HasCommitlintConfig reports the path of the repository's commitlint config,
// including the package.json key form. Detection is skipped entirely when none
// exists, so non-JS repositories pay nothing.
func HasCommitlintConfig(repoRoot string) (string, bool) {
	for _, name := range commitlintConfigNames {
		path := filepath.Join(repoRoot, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}

	pkg := filepath.Join(repoRoot, "package.json")
	data, err := os.ReadFile(pkg)
	if err != nil {
		return "", false
	}
	var probe struct {
		Commitlint json.RawMessage `json:"commitlint"`
	}
	if err := json.Unmarshal(data, &probe); err != nil || len(probe.Commitlint) == 0 {
		return "", false
	}
	return pkg, true
}

// lockfileRunners maps a repository-root lockfile to its runner invocation.
// --no-install on the npx path prevents a surprise network install.
var lockfileRunners = []struct {
	lockfile string
	name     string
	argv     []string
}{
	{"bun.lock", "bun", []string{"bun", "x", "commitlint"}},
	{"bun.lockb", "bun", []string{"bun", "x", "commitlint"}},
	{"pnpm-lock.yaml", "pnpm", []string{"pnpm", "exec", "commitlint"}},
	{"yarn.lock", "yarn", []string{"yarn", "commitlint"}},
	{"package-lock.json", "npx", []string{"npx", "--no-install", "commitlint"}},
}

// DetectRunner picks a commitlint invocation for repoRoot. It returns NoRunner
// when the integration is disabled, when commitlint.runner is "none", or when
// the repository declares no commitlint config.
func DetectRunner(repoRoot string, cfg CommitlintConfig) Runner {
	if !cfg.Enabled || cfg.Runner == "none" {
		return NoRunner
	}
	if _, ok := HasCommitlintConfig(repoRoot); !ok {
		return NoRunner
	}

	if cfg.Runner != "" && cfg.Runner != "auto" {
		for _, lr := range lockfileRunners {
			if lr.name == cfg.Runner {
				return Runner{Name: lr.name, Argv: slices.Clone(lr.argv)}
			}
		}
		return NoRunner
	}

	for _, lr := range lockfileRunners {
		if _, err := os.Stat(filepath.Join(repoRoot, lr.lockfile)); err == nil {
			return Runner{Name: lr.name, Argv: slices.Clone(lr.argv)}
		}
	}
	return NoRunner
}

// Constraints are the hard limits commitlint imposes. Every field is a pointer
// so an unset rule is distinguishable from a rule set to a zero value.
type Constraints struct {
	Types             *[]string
	Scopes            *[]string
	ScopeRequired     *bool
	HeaderMaxLength   *int
	BodyMaxLineLength *int
	TypeCase          *Case
	SubjectCase       *Case
	SubjectFullStop   *bool
}

// Override records one preference commitlint overruled, so gitia can name the
// rule that won.
type Override struct {
	Rule string
	From string
	To   string
}

// printConfig is the shape of `commitlint --print-config json`. Each rule is
// [level, applicable, value].
type printConfig struct {
	Rules map[string][]json.RawMessage `json:"rules"`
}

// ParsePrintConfig maps commitlint's materialized ruleset onto Constraints.
// Only level-2 (error) rules become constraints: levels 0 and 1 do not fail a
// commit, so treating them as hard limits would over-constrain the model.
func ParsePrintConfig(data []byte) (Constraints, error) {
	var pc printConfig
	if err := json.Unmarshal(data, &pc); err != nil {
		return Constraints{}, fmt.Errorf("parse commitlint --print-config json: %w", err)
	}

	var c Constraints
	for name, raw := range pc.Rules {
		level, applicable, value, ok := decodeRule(raw)
		if !ok || level != 2 {
			continue
		}

		switch name {
		case "type-enum":
			if v, ok := decodeStrings(value); ok && applicable == "always" {
				c.Types = &v
			}
		case "scope-enum":
			if v, ok := decodeStrings(value); ok && applicable == "always" {
				c.Scopes = &v
			}
		case "scope-empty":
			if applicable == "never" {
				t := true
				c.ScopeRequired = &t
			}
		case "header-max-length":
			if v, ok := decodeInt(value); ok && applicable == "always" {
				c.HeaderMaxLength = &v
			}
		case "body-max-line-length":
			if v, ok := decodeInt(value); ok && applicable == "always" {
				c.BodyMaxLineLength = &v
			}
		case "type-case":
			if v, ok := decodeString(value); ok && applicable == "always" && v == string(CaseLower) {
				k := CaseLower
				c.TypeCase = &k
			}
		case "subject-case":
			// The conventional config uses a `never` list of disallowed cases,
			// which does not map onto a single required case. Only the
			// `always "lower-case"` form becomes a constraint.
			if v, ok := decodeString(value); ok && applicable == "always" && v == string(CaseLower) {
				k := CaseLower
				c.SubjectCase = &k
			}
		case "subject-full-stop":
			if applicable == "never" {
				t := true
				c.SubjectFullStop = &t
			}
		}
	}
	return c, nil
}

func decodeRule(raw []json.RawMessage) (level int, applicable string, value json.RawMessage, ok bool) {
	if len(raw) == 0 {
		return 0, "", nil, false
	}
	if err := json.Unmarshal(raw[0], &level); err != nil {
		return 0, "", nil, false
	}
	if len(raw) > 1 {
		_ = json.Unmarshal(raw[1], &applicable)
	}
	if len(raw) > 2 {
		value = raw[2]
	}
	return level, applicable, value, true
}

func decodeStrings(raw json.RawMessage) ([]string, bool) {
	var v []string
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil || len(v) == 0 {
		return nil, false
	}
	return v, true
}

func decodeString(raw json.RawMessage) (string, bool) {
	var v string
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return "", false
	}
	return v, true
}

func decodeInt(raw json.RawMessage) (int, bool) {
	var v int
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return 0, false
	}
	return v, true
}

// Apply enforces c on top of rs and reports every preference it overruled.
// commitlint is not a precedence layer: it is a hard validity gate across all
// of them. Honoring a style preference that produces a commit the repository's
// own hook rejects would be worse than silently narrowing the preference, so
// gitia conforms and names the rule that won.
func (c Constraints) Apply(rs RuleSet) (RuleSet, []Override) {
	out := rs.Clone()
	var overrides []Override

	if c.Types != nil {
		allowed := *c.Types
		kept := make([]string, 0, len(out.Types))
		var dropped []string
		for _, t := range out.Types {
			if slices.Contains(allowed, t) {
				kept = append(kept, t)
			} else {
				dropped = append(dropped, t)
			}
		}
		if len(kept) == 0 {
			// Nothing the layers preferred is legal; adopt the enum verbatim.
			kept = slices.Clone(allowed)
		}
		if len(dropped) > 0 {
			overrides = append(overrides, Override{
				Rule: "type-enum",
				From: strings.Join(dropped, ", "),
				To:   strings.Join(kept, ", "),
			})
		}
		out.Types = kept
	}

	if c.Scopes != nil {
		if len(out.Scopes) > 0 {
			var dropped []string
			for _, s := range out.Scopes {
				if !slices.Contains(*c.Scopes, s) {
					dropped = append(dropped, s)
				}
			}
			if len(dropped) > 0 {
				overrides = append(overrides, Override{
					Rule: "scope-enum",
					From: strings.Join(dropped, ", "),
					To:   strings.Join(*c.Scopes, ", "),
				})
			}
		}
		out.Scopes = slices.Clone(*c.Scopes)
	}

	if c.ScopeRequired != nil && *c.ScopeRequired && !out.ScopeRequired {
		out.ScopeRequired = true
		overrides = append(overrides, Override{Rule: "scope-empty", From: "optional", To: "required"})
	}

	if c.HeaderMaxLength != nil && (out.HeaderMaxLength == 0 || *c.HeaderMaxLength < out.HeaderMaxLength) {
		if out.HeaderMaxLength != 0 {
			overrides = append(overrides, Override{
				Rule: "header-max-length",
				From: fmt.Sprint(out.HeaderMaxLength),
				To:   fmt.Sprint(*c.HeaderMaxLength),
			})
		}
		out.HeaderMaxLength = *c.HeaderMaxLength
	}

	if c.BodyMaxLineLength != nil && (out.BodyMaxLineLength == 0 || *c.BodyMaxLineLength < out.BodyMaxLineLength) {
		if out.BodyMaxLineLength != 0 {
			overrides = append(overrides, Override{
				Rule: "body-max-line-length",
				From: fmt.Sprint(out.BodyMaxLineLength),
				To:   fmt.Sprint(*c.BodyMaxLineLength),
			})
		}
		out.BodyMaxLineLength = *c.BodyMaxLineLength
	}

	if c.TypeCase != nil {
		out.TypeCase = *c.TypeCase
	}
	if c.SubjectCase != nil {
		out.SubjectCase = *c.SubjectCase
	}
	if c.SubjectFullStop != nil {
		out.SubjectFullStop = *c.SubjectFullStop
	}

	slices.SortFunc(overrides, func(a, b Override) int { return strings.Compare(a.Rule, b.Rule) })
	return out, overrides
}

// PrintConfig shells out to commitlint and returns its materialized ruleset.
// commitlint configs are usually JavaScript that `extends` a package inside
// node_modules; a Go binary can read those files but cannot evaluate them, so
// gitia uses commitlint as its own config oracle.
func PrintConfig(ctx context.Context, repoRoot string, r Runner) ([]byte, error) {
	if r.IsZero() {
		return nil, exitcode.Wrapf(exitcode.Generic, "no commitlint runner available")
	}

	args := append(slices.Clone(r.Argv[1:]), "--print-config", "json")
	cmd := exec.CommandContext(ctx, r.Argv[0], args...)
	cmd.Dir = repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, exitcode.Wrapf(exitcode.Generic,
			"%s --print-config json: %s", r.Name, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// cacheKey hashes the runner plus the config and lockfile mtimes, so the
// ~1-2 s Node startup is paid once rather than on every commit.
func cacheKey(repoRoot string, r Runner) string {
	h := sha256.New()
	fmt.Fprintf(h, "v1\n%s\n%s\n", repoRoot, r.Name)

	paths := []string{}
	if cfg, ok := HasCommitlintConfig(repoRoot); ok {
		paths = append(paths, cfg)
	}
	for _, lr := range lockfileRunners {
		paths = append(paths, filepath.Join(repoRoot, lr.lockfile))
	}
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil {
			fmt.Fprintf(h, "%s\t%d\t%d\n", p, info.ModTime().UnixNano(), info.Size())
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func cachePath(cacheDir, key string) string {
	return filepath.Join(cacheDir, "commitlint", key+".json")
}

// CachedPrintConfig returns the materialized ruleset, reading a warm cache
// entry when the config and lockfile are unchanged.
func CachedPrintConfig(ctx context.Context, repoRoot string, r Runner, cacheDir string) ([]byte, error) {
	if r.IsZero() {
		return nil, exitcode.Wrapf(exitcode.Generic, "no commitlint runner available")
	}

	path := cachePath(cacheDir, cacheKey(repoRoot, r))
	if data, err := os.ReadFile(path); err == nil {
		return data, nil
	}

	data, err := PrintConfig(ctx, repoRoot, r)
	if err != nil {
		return nil, err
	}

	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr == nil {
		// A cache write failure is not worth failing the commit over.
		_ = os.WriteFile(path, data, 0o644)
	}
	return data, nil
}

// WriteCacheForTest primes the cache. It exists so tests can assert cache hits
// without running Node; production code never calls it.
func WriteCacheForTest(repoRoot string, r Runner, cacheDir string, data []byte) error {
	path := cachePath(cacheDir, cacheKey(repoRoot, r))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// CommitlintEdit pipes a rendered message through the repository's own
// commitlint, which is the same check its git hook performs.
func CommitlintEdit(ctx context.Context, repoRoot string, r Runner, message string) error {
	if r.IsZero() {
		return nil
	}

	f, err := os.CreateTemp("", "gitia-commitlint-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(message); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	args := append(slices.Clone(r.Argv[1:]), "--edit", f.Name())
	cmd := exec.CommandContext(ctx, r.Argv[0], args...)
	cmd.Dir = repoRoot

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return exitcode.Wrapf(exitcode.CommitlintRejected,
			"commitlint rejected the message:\n%s", strings.TrimSpace(out.String()))
	}
	return nil
}
