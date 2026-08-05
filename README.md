# gitia

gitia generates a Conventional Commits message from your staged diff using a
model that runs **entirely on your machine** under [Ollama](https://ollama.com/).
Nothing — not the diff, not the message — ever leaves your computer. It knows
your repository's conventions (commitlint config, `AGENTS.md`, and more),
respects them as hard constraints rather than suggestions, and lets you confirm,
edit, regenerate, or abort before anything is committed.

## Install

Homebrew (macOS and Linux):

```bash
brew tap guerrero/homebrew-tap
brew install gitia
```

Or build from source:

```bash
go install github.com/guerrero/gitia/cmd/gitia@latest
```

gitia needs an Ollama server (`ollama serve`) with the default model
`gemma4:e2b-it-qat` — or run `gitia doctor` first to see what your machine is
missing.

## Quickstart

```bash
git add -p        # stage the changes you want to commit
gitia commit
```

gitia reads the staged diff, generates a message, and shows it with a
confirmation menu. `-y` skips the menu and commits directly; `-n` prints the
message without committing.

## Flags

| Flag | Short | Meaning |
| --- | --- | --- |
| `--yes` | `-y` | skip the confirmation prompt |
| `--model` | `-m` | override the model for this run |
| `--breaking` | `-b` | mark as a breaking change |
| `--fixes` | `-f` | issue numbers appended as `Fixes #123, #456` |
| `--dry-run` | `-n` | render and print, do not commit |
| `--type` | | force the commit type |
| `--scope` | | force the scope |
| `--no-body` | | subject line only |
| `--verbose` | | show resolved rules, prompt, and raw model output |

`--verbose` has no short form: `-v` is bound to `--version` on the root command.

## Configuration

`config.toml` lives in `$GITIA_CONFIG_DIR`, `$XDG_CONFIG_HOME/gitia`, or
`~/.config/gitia` (in that order) on both macOS and Linux. A missing file is
never an error — every value has a default:

```toml
[model]
name        = "gemma4:e2b-it-qat"
fallback    = ["gemma4:e2b", "gemma3n:e2b"]
temperature = 0.2
num_ctx     = 8192
keep_alive  = "5m"

[ollama]
host    = "http://localhost:11434"   # OLLAMA_HOST takes precedence
timeout = "60s"

[commit]
types = ["feat", "fix", "docs", "style", "refactor", "perf",
         "test", "build", "ci", "chore", "revert"]
header_max_length    = 72   # whole "type(scope): subject" line, as commitlint counts it
body_max_line_length = 100
body      = true
sign_off  = false
language  = "en"

[commitlint]
enabled = true
runner  = "auto"   # auto | npx | pnpm | yarn | bun | none

[agents]
enabled   = true
files     = ["AGENTS.md", "CLAUDE.md", "GEMINI.md"]
max_bytes = 2000

[diff]
max_bytes = 32768
exclude   = ["*.lock", "package-lock.json", "pnpm-lock.yaml",
             "go.sum", "*.min.js", "dist/**"]
```

## gitia doctor

`gitia doctor` checks the ten things that can stop gitia from working:

```text
ok    git              git version 2.54.0
ok    repository       /Users/alex/conductor/workspaces/gitia/montreal
FAIL  ollama binary    ollama not found on PATH; install it with: brew install ollama
ok    ollama server    http://localhost:11434 (1 models)
FAIL  model            gemma4:e2b-it-qat not present; run: ollama pull gemma4:e2b-it-qat
warn  node             /Users/alex/.local/share/pnpm/node found, but no package manager lockfile in this repository
warn  commitlint       no commitlint config in this repository
warn  config           /Users/alex/.config/gitia/config.toml not present; using defaults
warn  editor           neither EDITOR nor VISUAL is set; [e] will use vi
ok    conventions      [AGENTS.md]
gitia: one or more checks failed
```

A `--json` mode prints the same checks machine-readably.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | generic error |
| 2 | usage error |
| 3 | not a git repository |
| 4 | nothing staged |
| 5 | Ollama unreachable |
| 6 | model missing and cannot be pulled non-interactively |
| 7 | generation failed after retry |
| 8 | commitlint rejected the message |
| 130 | aborted by the user |

## How it works

**Rule precedence.** The commit rules start from the Conventional Commits
defaults, then each layer overrides the one below: `config.toml`, then the
`AGENTS.md`/`CLAUDE.md`/`GEMINI.md` files touched by your staged diff, then CLI
flags. gitia also resolves your repository's commitlint config through
`commitlint --print-config json` (cached on the config and lockfile mtimes) so
the generated message obeys the rules commitlint would enforce.

**The commitlint gate.** commitlint is a hard validity gate, not a suggestion:
the generated message must pass your repository's own commitlint rules before
gitia will commit it. The JSON schema sent to Ollama is derived from the same
resolved rules, so the model cannot produce an illegal commit type in the first
place.

**Small diffs, complete context.** The staged `--stat` always reaches the model.
The full diff is included up to `diff.max_bytes`; over budget, excluded paths
drop to stat-only first, then the largest files lose their hunk bodies — while
every changed path stays listed, so the model never invents a scope for a file
it did not know about.
