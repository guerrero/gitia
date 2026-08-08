package rules_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/guerrero/gitia/internal/rules"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultConfig(t *testing.T) {
	cfg := rules.DefaultConfig()

	if cfg.Model.Name != "gemma4:e2b-it-qat" {
		t.Errorf("Model.Name = %q, want %q", cfg.Model.Name, "gemma4:e2b-it-qat")
	}
	if len(cfg.Model.Fallback) != 2 || cfg.Model.Fallback[0] != "gemma4:e2b" {
		t.Errorf("Model.Fallback = %v, want [gemma4:e2b gemma3n:e2b]", cfg.Model.Fallback)
	}
	if cfg.Model.Temperature != 0.2 {
		t.Errorf("Model.Temperature = %v, want 0.2", cfg.Model.Temperature)
	}
	if cfg.Model.NumCtx != 8192 {
		t.Errorf("Model.NumCtx = %d, want 8192", cfg.Model.NumCtx)
	}
	if cfg.Ollama.Host != "http://localhost:11434" {
		t.Errorf("Ollama.Host = %q, want %q", cfg.Ollama.Host, "http://localhost:11434")
	}
	if !cfg.Commit.Body {
		t.Error("Commit.Body = false, want true")
	}
	if got := cfg.Commit.HeaderIdealLength; len(got) != 2 || got[0] != 50 || got[1] != 55 {
		t.Errorf("Commit.HeaderIdealLength = %v, want [50 55]", got)
	}
	if !cfg.Commitlint.Enabled || cfg.Commitlint.Runner != "auto" {
		t.Errorf("Commitlint = %+v, want enabled with runner auto", cfg.Commitlint)
	}
	if cfg.Agents.MaxBytes != 2000 {
		t.Errorf("Agents.MaxBytes = %d, want 2000", cfg.Agents.MaxBytes)
	}
	if cfg.Diff.MaxBytes != 32768 {
		t.Errorf("Diff.MaxBytes = %d, want 32768", cfg.Diff.MaxBytes)
	}
	if len(cfg.Diff.Exclude) != 6 {
		t.Errorf("Diff.Exclude = %v, want 6 default patterns", cfg.Diff.Exclude)
	}
}

func TestLoadConfigMissingFileIsNotAnError(t *testing.T) {
	cfg, found, err := rules.LoadConfigAt(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatalf("LoadConfigAt: %v", err)
	}
	if found {
		t.Error("found = true for a missing file, want false")
	}
	if cfg.Model.Name != rules.DefaultConfig().Model.Name {
		t.Error("a missing file must yield the defaults")
	}
}

func TestLoadConfigPartialFileKeepsOtherDefaults(t *testing.T) {
	path := writeConfig(t, `
[model]
name = "qwen3:4b"

[commit]
header_max_length = 50
`)

	cfg, found, err := rules.LoadConfigAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("found = false for an existing file, want true")
	}
	if cfg.Model.Name != "qwen3:4b" {
		t.Errorf("Model.Name = %q, want %q", cfg.Model.Name, "qwen3:4b")
	}
	if cfg.Model.Temperature != 0.2 {
		t.Errorf("Model.Temperature = %v; an absent key must keep its default", cfg.Model.Temperature)
	}
	if cfg.Commit.HeaderMaxLength != 50 {
		t.Errorf("Commit.HeaderMaxLength = %d, want 50", cfg.Commit.HeaderMaxLength)
	}
	if cfg.Commit.BodyMaxLineLength != 72 {
		t.Errorf("Commit.BodyMaxLineLength = %d; an absent key must keep its default", cfg.Commit.BodyMaxLineLength)
	}
}

func TestLoadConfigParsesHeaderIdealLength(t *testing.T) {
	path := writeConfig(t, "[commit]\nheader_ideal_length = [50, 55]\n")

	cfg, _, err := rules.LoadConfigAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Commit.HeaderIdealLength) != 2 ||
		cfg.Commit.HeaderIdealLength[0] != 50 || cfg.Commit.HeaderIdealLength[1] != 55 {
		t.Errorf("Commit.HeaderIdealLength = %v, want [50 55]", cfg.Commit.HeaderIdealLength)
	}
}

func TestLoadConfigHonorsExplicitFalse(t *testing.T) {
	path := writeConfig(t, "[commit]\nbody = false\n\n[commitlint]\nenabled = false\n")

	cfg, _, err := rules.LoadConfigAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Commit.Body {
		t.Error("Commit.Body = true; an explicit `body = false` must win over the default")
	}
	if cfg.Commitlint.Enabled {
		t.Error("Commitlint.Enabled = true; an explicit `enabled = false` must win over the default")
	}
}

func TestLoadConfigRejectsMalformedTOML(t *testing.T) {
	path := writeConfig(t, "[model\nname = \n")
	if _, _, err := rules.LoadConfigAt(path); err == nil {
		t.Fatal("LoadConfigAt() = nil error for malformed TOML, want an error")
	}
}

func TestConfigApply(t *testing.T) {
	cfg := rules.DefaultConfig()
	cfg.Commit.Types = []string{"feat", "fix"}
	cfg.Commit.HeaderMaxLength = 50
	cfg.Commit.BodyMaxLineLength = 72
	cfg.Commit.HeaderIdealLength = []int{40, 60}
	cfg.Commit.Body = false
	cfg.Commit.SignOff = true
	cfg.Commit.Language = "es"

	rs := cfg.Apply(rules.Conventional())

	if len(rs.Types) != 2 || rs.Types[0] != "feat" {
		t.Errorf("Types = %v, want [feat fix]", rs.Types)
	}
	if rs.HeaderMaxLength != 50 {
		t.Errorf("HeaderMaxLength = %d, want 50", rs.HeaderMaxLength)
	}
	if rs.BodyMaxLineLength != 72 {
		t.Errorf("BodyMaxLineLength = %d, want 72", rs.BodyMaxLineLength)
	}
	if rs.HeaderIdealMin != 40 || rs.HeaderIdealMax != 60 {
		t.Errorf("HeaderIdeal = [%d %d], want [40 60]", rs.HeaderIdealMin, rs.HeaderIdealMax)
	}
	if rs.IncludeBody {
		t.Error("IncludeBody = true, want false")
	}
	if !rs.SignOff {
		t.Error("SignOff = false, want true")
	}
	if rs.Language != "es" {
		t.Errorf("Language = %q, want %q", rs.Language, "es")
	}
}

func TestConfigApplyEmptyTypesKeepsTheBaseline(t *testing.T) {
	cfg := rules.DefaultConfig()
	cfg.Commit.Types = nil

	rs := cfg.Apply(rules.Conventional())
	if len(rs.Types) != len(rules.Conventional().Types) {
		t.Errorf("Types = %v; an empty commit.types must leave the baseline intact", rs.Types)
	}
}

func TestConfigApplyHeaderIdealLengthOverlay(t *testing.T) {
	base := rules.Conventional()

	// An empty or 1-element slice means "not configured": baseline stays.
	for _, v := range [][]int{nil, {}, {55}} {
		cfg := rules.DefaultConfig()
		cfg.Commit.HeaderIdealLength = v
		rs := cfg.Apply(base)
		if rs.HeaderIdealMin != 50 || rs.HeaderIdealMax != 55 {
			t.Errorf("HeaderIdealLength = %v must keep the baseline, got [%d %d]",
				v, rs.HeaderIdealMin, rs.HeaderIdealMax)
		}
	}

	// [0, 0] explicitly disables the guidance.
	cfg := rules.DefaultConfig()
	cfg.Commit.HeaderIdealLength = []int{0, 0}
	rs := cfg.Apply(base)
	if rs.HeaderIdealMin != 0 || rs.HeaderIdealMax != 0 {
		t.Errorf("HeaderIdealLength = [0 0] must disable the guidance, got [%d %d]",
			rs.HeaderIdealMin, rs.HeaderIdealMax)
	}

	// Extras beyond the first two are ignored.
	cfg = rules.DefaultConfig()
	cfg.Commit.HeaderIdealLength = []int{70, 80, 90}
	rs = cfg.Apply(base)
	if rs.HeaderIdealMin != 70 || rs.HeaderIdealMax != 80 {
		t.Errorf("HeaderIdealLength = [70 80 90] must use the first two, got [%d %d]",
			rs.HeaderIdealMin, rs.HeaderIdealMax)
	}
}

func TestConfigDirPrecedence(t *testing.T) {
	t.Setenv("GITIA_CONFIG_DIR", "/explicit/gitia")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	dir, err := rules.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/explicit/gitia" {
		t.Errorf("ConfigDir() = %q, want %q (GITIA_CONFIG_DIR wins)", dir, "/explicit/gitia")
	}

	t.Setenv("GITIA_CONFIG_DIR", "")
	if dir, err = rules.ConfigDir(); err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join("/xdg", "gitia") {
		t.Errorf("ConfigDir() = %q, want %q (XDG_CONFIG_HOME is next)", dir, "/xdg/gitia")
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	if dir, err = rules.ConfigDir(); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, ".config", "gitia"); dir != want {
		t.Errorf("ConfigDir() = %q, want %q (~/.config/gitia on both macOS and Linux)", dir, want)
	}
}

func TestOllamaHostEnvironmentWins(t *testing.T) {
	cfg := rules.DefaultConfig()
	cfg.Ollama.Host = "http://from-config:1234"

	t.Setenv("OLLAMA_HOST", "")
	if got := cfg.OllamaHost(); got != "http://from-config:1234" {
		t.Errorf("OllamaHost() = %q, want the config value", got)
	}

	t.Setenv("OLLAMA_HOST", "http://from-env:9999")
	if got := cfg.OllamaHost(); got != "http://from-env:9999" {
		t.Errorf("OllamaHost() = %q, want the environment value", got)
	}
}

func TestOllamaHostNormalizesABareHostPort(t *testing.T) {
	cfg := rules.DefaultConfig()
	t.Setenv("OLLAMA_HOST", "127.0.0.1:11434")

	if got := cfg.OllamaHost(); got != "http://127.0.0.1:11434" {
		t.Errorf("OllamaHost() = %q, want a scheme to be added", got)
	}
}

func TestOllamaTimeout(t *testing.T) {
	cfg := rules.DefaultConfig()
	if got := cfg.OllamaTimeout().String(); got != "1m0s" {
		t.Errorf("OllamaTimeout() = %v, want 60s", got)
	}

	cfg.Ollama.Timeout = "nonsense"
	if got := cfg.OllamaTimeout().String(); got != "1m0s" {
		t.Errorf("OllamaTimeout() = %v; an unparseable value must fall back to 60s", got)
	}
}
