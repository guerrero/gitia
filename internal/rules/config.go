package rules

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config mirrors ~/.config/gitia/config.toml. Every field has a default, so a
// missing file is never an error.
type Config struct {
	Model      ModelConfig      `toml:"model"`
	Ollama     OllamaConfig     `toml:"ollama"`
	Commit     CommitConfig     `toml:"commit"`
	Commitlint CommitlintConfig `toml:"commitlint"`
	Agents     AgentsConfig     `toml:"agents"`
	Diff       DiffConfig       `toml:"diff"`
}

type ModelConfig struct {
	Name        string   `toml:"name"`
	Fallback    []string `toml:"fallback"`
	Temperature float64  `toml:"temperature"`
	NumCtx      int      `toml:"num_ctx"`
	KeepAlive   string   `toml:"keep_alive"`
}

type OllamaConfig struct {
	Host    string `toml:"host"`
	Timeout string `toml:"timeout"`
}

type CommitConfig struct {
	Types             []string `toml:"types"`
	HeaderMaxLength   int      `toml:"header_max_length"`
	BodyMaxLineLength int      `toml:"body_max_line_length"`
	Body              bool     `toml:"body"`
	SignOff           bool     `toml:"sign_off"`
	Language          string   `toml:"language"`
}

type CommitlintConfig struct {
	Enabled bool   `toml:"enabled"`
	Runner  string `toml:"runner"`
}

type AgentsConfig struct {
	Enabled  bool     `toml:"enabled"`
	Files    []string `toml:"files"`
	MaxBytes int      `toml:"max_bytes"`
}

type DiffConfig struct {
	MaxBytes int      `toml:"max_bytes"`
	Exclude  []string `toml:"exclude"`
}

const defaultOllamaTimeout = 60 * time.Second

// DefaultConfig returns the configuration gitia uses when no file exists.
func DefaultConfig() Config {
	base := Conventional()
	return Config{
		Model: ModelConfig{
			Name:        "gemma4:e2b-it-qat",
			Fallback:    []string{"gemma4:e2b", "gemma3n:e2b"},
			Temperature: 0.2,
			NumCtx:      8192,
			KeepAlive:   "5m",
		},
		Ollama: OllamaConfig{
			Host:    "http://localhost:11434",
			Timeout: "60s",
		},
		Commit: CommitConfig{
			Types:             base.Types,
			HeaderMaxLength:   base.HeaderMaxLength,
			BodyMaxLineLength: base.BodyMaxLineLength,
			Body:              true,
			SignOff:           false,
			Language:          "en",
		},
		Commitlint: CommitlintConfig{Enabled: true, Runner: "auto"},
		Agents: AgentsConfig{
			Enabled:  true,
			Files:    []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"},
			MaxBytes: 2000,
		},
		Diff: DiffConfig{
			MaxBytes: 32768,
			Exclude: []string{
				"*.lock", "package-lock.json", "pnpm-lock.yaml",
				"go.sum", "*.min.js", "dist/**",
			},
		},
	}
}

// ConfigDir resolves gitia's configuration directory. The XDG layout is used
// on macOS as well as Linux: CLI convention beats ~/Library/Application Support.
func ConfigDir() (string, error) {
	if dir := os.Getenv("GITIA_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "gitia"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".config", "gitia"), nil
}

// ConfigPath returns the full path to config.toml.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// LoadConfig reads path, falling back to defaults when it does not exist.
func LoadConfig(path string) (Config, error) {
	cfg, _, err := LoadConfigAt(path)
	return cfg, err
}

// LoadConfigAt reads path and reports whether the file existed. Decoding
// starts from the defaults, and BurntSushi/toml overwrites only the keys the
// file actually contains, so `body = false` is honored and an absent key keeps
// its default.
func LoadConfigAt(path string) (Config, bool, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, false, fmt.Errorf("read %s: %w", path, err)
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return DefaultConfig(), true, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, true, nil
}

// Apply overlays the [commit] section onto rs. Empty values mean "not
// configured" and leave the layer below untouched.
func (c Config) Apply(rs RuleSet) RuleSet {
	out := rs.Clone()

	if len(c.Commit.Types) > 0 {
		out.Types = append([]string(nil), c.Commit.Types...)
	}
	if c.Commit.HeaderMaxLength > 0 {
		out.HeaderMaxLength = c.Commit.HeaderMaxLength
	}
	if c.Commit.BodyMaxLineLength > 0 {
		out.BodyMaxLineLength = c.Commit.BodyMaxLineLength
	}
	if c.Commit.Language != "" {
		out.Language = c.Commit.Language
	}
	out.IncludeBody = c.Commit.Body
	out.SignOff = c.Commit.SignOff

	return out
}

// OllamaHost returns the base URL for the Ollama server. OLLAMA_HOST takes
// precedence over the config file, matching the ollama CLI. A bare host:port
// is normalized to an http:// URL.
func (c Config) OllamaHost() string {
	host := c.Ollama.Host
	if env := strings.TrimSpace(os.Getenv("OLLAMA_HOST")); env != "" {
		host = env
	}
	if host == "" {
		host = "http://localhost:11434"
	}
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}
	return strings.TrimRight(host, "/")
}

// OllamaTimeout parses ollama.timeout, falling back to 60s when it is absent
// or unparseable. A bad duration should not stop gitia from running.
func (c Config) OllamaTimeout() time.Duration {
	d, err := time.ParseDuration(c.Ollama.Timeout)
	if err != nil || d <= 0 {
		return defaultOllamaTimeout
	}
	return d
}

// KeepAlive returns the model keep-alive window sent to Ollama.
func (c Config) KeepAlive() string {
	if c.Model.KeepAlive == "" {
		return "5m"
	}
	return c.Model.KeepAlive
}
