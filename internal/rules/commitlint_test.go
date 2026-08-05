package rules_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/guerrero/gitia/internal/rules"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "commitlint", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParsePrintConfigConventional(t *testing.T) {
	c, err := rules.ParsePrintConfig(fixture(t, "config-conventional.json"))
	if err != nil {
		t.Fatalf("ParsePrintConfig: %v", err)
	}

	if c.Types == nil || len(*c.Types) != 11 {
		t.Errorf("Types = %v, want the 11 conventional types", c.Types)
	}
	if c.HeaderMaxLength == nil || *c.HeaderMaxLength != 100 {
		t.Errorf("HeaderMaxLength = %v, want 100", c.HeaderMaxLength)
	}
	if c.BodyMaxLineLength == nil || *c.BodyMaxLineLength != 100 {
		t.Errorf("BodyMaxLineLength = %v, want 100", c.BodyMaxLineLength)
	}
	if c.TypeCase == nil || *c.TypeCase != rules.CaseLower {
		t.Errorf("TypeCase = %v, want lower-case", c.TypeCase)
	}
	if c.SubjectFullStop == nil || !*c.SubjectFullStop {
		t.Errorf("SubjectFullStop = %v, want true", c.SubjectFullStop)
	}
	if c.SubjectCase != nil {
		t.Errorf("SubjectCase = %v; a `never` list of cases must not map to a case constraint", c.SubjectCase)
	}
	if c.Scopes != nil {
		t.Errorf("Scopes = %v, want nil when scope-enum is absent", c.Scopes)
	}
	if c.ScopeRequired != nil {
		t.Errorf("ScopeRequired = %v, want nil when scope-empty is absent", c.ScopeRequired)
	}
}

func TestParsePrintConfigIgnoresNonErrorLevels(t *testing.T) {
	c, err := rules.ParsePrintConfig(fixture(t, "restrictive.json"))
	if err != nil {
		t.Fatal(err)
	}

	if c.BodyMaxLineLength != nil {
		t.Errorf("BodyMaxLineLength = %v; a level-1 warning must not become a hard constraint", c.BodyMaxLineLength)
	}
	if c.SubjectCase != nil {
		t.Errorf("SubjectCase = %v; a level-0 rule is disabled and must be ignored", c.SubjectCase)
	}
	if c.ScopeRequired == nil || !*c.ScopeRequired {
		t.Errorf("ScopeRequired = %v, want true from `scope-empty: [2, never]`", c.ScopeRequired)
	}
}

func TestParsePrintConfigRejectsGarbage(t *testing.T) {
	if _, err := rules.ParsePrintConfig([]byte("not json")); err == nil {
		t.Fatal("ParsePrintConfig() = nil error for non-JSON input, want an error")
	}
}

func TestConstraintsApplyOverridesAndReports(t *testing.T) {
	c, err := rules.ParsePrintConfig(fixture(t, "restrictive.json"))
	if err != nil {
		t.Fatal(err)
	}

	// A RuleSet where AGENTS.md asked for chore, which commitlint forbids.
	rs := rules.Conventional()
	rs.Types = []string{"chore", "feat", "fix"}

	got, overrides := c.Apply(rs)

	if len(got.Types) != 2 || got.Types[0] != "feat" || got.Types[1] != "fix" {
		t.Errorf("Types = %v, want [feat fix]", got.Types)
	}
	if len(got.Scopes) != 3 {
		t.Errorf("Scopes = %v, want the three allowed scopes", got.Scopes)
	}
	if !got.ScopeRequired {
		t.Error("ScopeRequired = false, want true")
	}
	if got.HeaderMaxLength != 72 {
		t.Errorf("HeaderMaxLength = %d, want 72", got.HeaderMaxLength)
	}

	var sawTypeEnum bool
	for _, o := range overrides {
		if o.Rule == "type-enum" {
			sawTypeEnum = true
			if o.From == "" || o.To == "" {
				t.Errorf("override = %+v, want both From and To populated", o)
			}
		}
	}
	if !sawTypeEnum {
		t.Errorf("overrides = %+v, want a type-enum entry naming the rule that won", overrides)
	}
}

func TestConstraintsApplyReportsNoOverrideWhenNothingChanges(t *testing.T) {
	c, err := rules.ParsePrintConfig(fixture(t, "config-conventional.json"))
	if err != nil {
		t.Fatal(err)
	}

	rs := rules.Conventional()
	rs.Types = []string{"feat", "fix"} // a strict subset of what commitlint allows

	got, overrides := c.Apply(rs)

	if len(got.Types) != 2 {
		t.Errorf("Types = %v; a subset of the commitlint enum must survive intact", got.Types)
	}
	for _, o := range overrides {
		if o.Rule == "type-enum" {
			t.Errorf("overrides = %+v, want no type-enum override when the preference is already legal", overrides)
		}
	}
}

func TestConstraintsApplyDropsEveryIllegalTypeButKeepsOne(t *testing.T) {
	c, err := rules.ParsePrintConfig(fixture(t, "restrictive.json"))
	if err != nil {
		t.Fatal(err)
	}

	rs := rules.Conventional()
	rs.Types = []string{"chore", "docs"} // neither is legal

	got, _ := c.Apply(rs)

	if len(got.Types) == 0 {
		t.Fatal("Types is empty; a RuleSet with no legal type cannot generate anything")
	}
	if got.Types[0] != "feat" {
		t.Errorf("Types = %v, want the commitlint enum verbatim when no preference survives", got.Types)
	}
}

func TestDetectRunner(t *testing.T) {
	tests := []struct {
		lockfile string
		wantName string
		wantArgv []string
	}{
		{"bun.lock", "bun", []string{"bun", "x", "commitlint"}},
		{"bun.lockb", "bun", []string{"bun", "x", "commitlint"}},
		{"pnpm-lock.yaml", "pnpm", []string{"pnpm", "exec", "commitlint"}},
		{"yarn.lock", "yarn", []string{"yarn", "commitlint"}},
		{"package-lock.json", "npx", []string{"npx", "--no-install", "commitlint"}},
	}

	for _, tt := range tests {
		t.Run(tt.lockfile, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "commitlint.config.js", "module.exports = {}\n")
			writeFile(t, root, tt.lockfile, "")

			r := rules.DetectRunner(root, rules.DefaultConfig().Commitlint)

			if r.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", r.Name, tt.wantName)
			}
			if len(r.Argv) != len(tt.wantArgv) {
				t.Fatalf("Argv = %v, want %v", r.Argv, tt.wantArgv)
			}
			for i := range tt.wantArgv {
				if r.Argv[i] != tt.wantArgv[i] {
					t.Errorf("Argv[%d] = %q, want %q", i, r.Argv[i], tt.wantArgv[i])
				}
			}
		})
	}
}

func TestDetectRunnerSkippedWithoutAConfigFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pnpm-lock.yaml", "")

	if r := rules.DetectRunner(root, rules.DefaultConfig().Commitlint); !r.IsZero() {
		t.Errorf("DetectRunner() = %+v; non-JS repositories must pay nothing", r)
	}
}

func TestDetectRunnerHonorsAnExplicitOverride(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "commitlint.config.js", "module.exports = {}\n")
	writeFile(t, root, "pnpm-lock.yaml", "")

	cfg := rules.DefaultConfig().Commitlint
	cfg.Runner = "bun"
	if r := rules.DetectRunner(root, cfg); r.Name != "bun" {
		t.Errorf("Name = %q, want bun from commitlint.runner", r.Name)
	}

	cfg.Runner = "none"
	if r := rules.DetectRunner(root, cfg); !r.IsZero() {
		t.Errorf("DetectRunner() = %+v, want the zero Runner when commitlint.runner is none", r)
	}

	cfg.Runner = "auto"
	cfg.Enabled = false
	if r := rules.DetectRunner(root, cfg); !r.IsZero() {
		t.Errorf("DetectRunner() = %+v, want the zero Runner when commitlint.enabled is false", r)
	}
}

func TestHasCommitlintConfigFindsEveryConventionalName(t *testing.T) {
	names := []string{
		"commitlint.config.js", "commitlint.config.mjs", "commitlint.config.cjs",
		"commitlint.config.ts", ".commitlintrc", ".commitlintrc.json",
		".commitlintrc.yml", ".commitlintrc.yaml", ".commitlintrc.js", ".commitlintrc.ts",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, name, "{}\n")

			path, ok := rules.HasCommitlintConfig(root)
			if !ok {
				t.Fatalf("HasCommitlintConfig() = false for %s", name)
			}
			if filepath.Base(path) != name {
				t.Errorf("path = %q, want it to end in %q", path, name)
			}
		})
	}
}

func TestHasCommitlintConfigFindsThePackageJSONKey(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"name":"x","commitlint":{"extends":["@commitlint/config-conventional"]}}`)

	if _, ok := rules.HasCommitlintConfig(root); !ok {
		t.Error("HasCommitlintConfig() = false for a package.json commitlint key")
	}
}

func TestHasCommitlintConfigIgnoresAPackageJSONWithoutTheKey(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"name":"x"}`)

	if _, ok := rules.HasCommitlintConfig(root); ok {
		t.Error("HasCommitlintConfig() = true for a package.json with no commitlint key")
	}
}

func TestCachedPrintConfigReusesAWarmEntry(t *testing.T) {
	root := t.TempDir()
	cache := t.TempDir()
	writeFile(t, root, "commitlint.config.js", "module.exports = {}\n")
	writeFile(t, root, "pnpm-lock.yaml", "")

	// A runner that would fail if executed; a cache hit must never reach it.
	broken := rules.Runner{Name: "test", Argv: []string{"false"}}

	want := fixture(t, "config-conventional.json")
	if err := rules.WriteCacheForTest(root, broken, cache, want); err != nil {
		t.Fatal(err)
	}

	got, err := rules.CachedPrintConfig(t.Context(), root, broken, cache)
	if err != nil {
		t.Fatalf("CachedPrintConfig: %v", err)
	}
	if string(got) != string(want) {
		t.Error("CachedPrintConfig() did not return the cached bytes")
	}
}

func TestCachedPrintConfigMissesWhenTheConfigChanges(t *testing.T) {
	root := t.TempDir()
	cache := t.TempDir()
	writeFile(t, root, "commitlint.config.js", "module.exports = {}\n")
	writeFile(t, root, "pnpm-lock.yaml", "")

	broken := rules.Runner{Name: "test", Argv: []string{"false"}}
	if err := rules.WriteCacheForTest(root, broken, cache, fixture(t, "config-conventional.json")); err != nil {
		t.Fatal(err)
	}

	// Rewriting the config changes its mtime, which is part of the cache key.
	writeFile(t, root, "commitlint.config.js", "module.exports = {rules:{}}\n")

	if _, err := rules.CachedPrintConfig(t.Context(), root, broken, cache); err == nil {
		t.Fatal("CachedPrintConfig() = nil error; a stale key must miss and then fail on the broken runner")
	}
}
