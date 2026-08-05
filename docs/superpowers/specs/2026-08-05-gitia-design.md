# gitia — Design

**Date:** 2026-08-05
**Status:** Approved
**Module:** `github.com/guerrero/gitia`

## Context

Writing good commit messages is tedious, and the result is usually worse than what a
model can infer from the diff. Existing AI commit tools send your diff to a hosted API,
which is a non-starter for private repositories and adds network latency to a local
operation.

gitia is a git CLI that generates Conventional Commits from the staged diff using a
model running locally under Ollama. Nothing leaves the machine. It respects the
conventions a repository already declares — `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, and
commitlint — rather than imposing its own.

The MVP is four commands: `help`, `--version`, `doctor`, `commit`. Only `commit` does
real work; the other three exist so the tool is diagnosable and installable from day one.

## Goals

- A single static binary for macOS and Linux, amd64 and arm64.
- Ollama and the model are **never** bundled. gitia is a client.
- Generated messages satisfy Conventional Commits v1.0.0 by default, and satisfy the
  repository's commitlint configuration when one is present.
- Repository conventions declared in `AGENTS.md` (and siblings) influence the output.
- Fast enough to sit in the normal commit loop on an M1/16 GB.

## Non-goals (MVP)

- Changelog generation, PR descriptions, commit amending, interactive rebase helpers.
- Hosted model providers.
- Windows support (the code should not preclude it; it is not tested or released).
- A `prepare-commit-msg` hook shim. The layout leaves room for one later.

---

## 1. Language and dependencies

Go 1.26, `CGO_ENABLED=0`, cross-compiled with `GOOS`/`GOARCH`.

| Dependency | Purpose |
| --- | --- |
| `spf13/cobra` | commands, `help`, man page generation, shell completions |
| `BurntSushi/toml` | config file parsing |
| `rogpeppe/go-internal/testscript` | end-to-end CLI tests (test-only) |

Everything else is the standard library: `os/exec` for git and commitlint, `net/http`
for Ollama, `encoding/json` for structured output.

Go was chosen over Rust and TypeScript/Bun because the stdlib already covers both
external integrations, cross-compilation to a true static binary is a build-tag away,
and cobra provides `man gitia` and completions for free. Bun's `--compile` embeds a
50–90 MB runtime; Rust would require pulling in tokio, reqwest, and clap to reach parity
with Go's stdlib.

## 2. Repository layout

```
cmd/gitia/main.go            thin — wire root command, map errors to exit codes
internal/cli/                cobra commands
  root.go  version.go  doctor.go  commit.go
internal/git/                exec git: repo root, staged diff, stat, name-status, commit
internal/ollama/             HTTP client: /api/tags, /api/show, /api/pull, /api/generate
internal/rules/              precedence engine
  conventional.go              Conventional Commits v1.0.0 baseline
  config.go                    ~/.config/gitia/config.toml
  agentsmd.go                  discovery + heading extraction
  commitlint.go                runner detection, --print-config json, cache
  resolve.go                   merge all layers into one RuleSet
internal/commit/             Message struct, render, validate, deterministic repair
internal/prompt/             system/user prompt construction, JSON schema builder
internal/ui/                 TTY detection, A/E/R/Q menu, $EDITOR handoff
man/gitia.1.tmpl             hand-written wrapper sections
man/gitia.1                  generated, committed
```

`internal/` is compiler-enforced, not stylistic: packages beneath it are importable only
by code rooted at the module. Since gitia is an application, everything except `main` is
private and owes no API compatibility. If `internal/rules/commitlint.go` later proves
reusable as a standalone Go commitlint-config resolver, moving that package to the repo
root is a one-commit change; the reverse would be breaking.

`internal/rules` is the only package that knows about precedence. `internal/commit`
validates against a `RuleSet` it is handed and knows nothing about where the rules came
from. That boundary makes the precedence matrix testable as a pure table test with no
git, no filesystem, and no Ollama.

`internal/prompt` depends on `internal/rules`, never the reverse — the JSON schema sent
to the model is *derived* from the resolved rules, so "what types are legal" has one
source of truth.

### Supporting files

```
README.md              install, quickstart, flags, config reference, sample doctor output
AGENTS.md              gitia's own commit conventions — dogfooded by every commit here
LICENSE                The Unlicense
CHANGELOG.md           keep-a-changelog format
CONTRIBUTING.md        dev setup, running tests without a local Ollama
.gitignore             /gitia, /dist/, *.test, coverage.out, .DS_Store
.editorconfig          tabs for .go; 2-space for .yml/.toml/.json/.md
Makefile               build test lint man install release-dry
.golangci.yml          govet, staticcheck, errcheck, revive, gofumpt
.goreleaser.yaml       darwin+linux × amd64+arm64, ldflags, Homebrew tap, man in archive
.github/workflows/ci.yml       test + lint + build matrix
.github/workflows/release.yml  goreleaser on tag
```

Shipping gitia's own `AGENTS.md` means the discovery path is exercised by every commit
made to the project, so the feature cannot silently rot.

## 3. Commands

### `gitia help` / `--help` / `-h`

Provided by cobra.

### `gitia --version` / `-v`

Prints version, git commit, build date, Go version, and `GOOS/GOARCH`, injected via
`-ldflags -X`. Note that `-v` is bound to version rather than the more common verbose;
`gitia commit --verbose` has no short form as a result.

### `gitia doctor`

Checks, in order, printing pass/warn/fail per line:

1. `git` on PATH, and its version
2. current directory is inside a git work tree
3. `ollama` binary on PATH
4. Ollama server reachable — `GET {host}/api/tags`
5. default model present locally, with its on-disk size
6. Node and package manager detected (for commitlint)
7. commitlint config detected, and `--print-config json` resolves
8. gitia config file located and parses
9. `$EDITOR` or `$VISUAL` set
10. `AGENTS.md` / `CLAUDE.md` / `GEMINI.md` discovered from the current directory

`--json` emits machine-readable results. Exit 0 when no check fails, 1 otherwise.
Checks 6–10 are warnings, not failures — gitia works without any of them.

### `gitia commit`

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

`--y`, `--m`, `--b`, `--f`, `--n` are not expressible in POSIX/pflag — a double dash
requires a long name — so the long forms above are canonical.

`--fixes` accepts three forms: `--fixes 123,456` (comma-separated), repeated
`--fixes 123 --fixes 456`, and `--fixes 123 456`. The third is not expressible in pflag
either, so a pre-parse pass over `os.Args` greedily absorbs bare numeric arguments
immediately following `--fixes`/`-f` and rewrites them into the comma-separated form
before cobra parses. Non-numeric arguments terminate the absorption.

## 4. Data flow

```
preflight    in a git repo? staged changes? ollama reachable? model present?
   ↓
collect      git diff --staged  +  --stat  +  --name-status
   ↓
resolve      Conventional Commits defaults
             → user config
             → repo-root AGENTS.md / CLAUDE.md / GEMINI.md
             → nested AGENTS.md nearest to each staged path
             → CLI flags
             ⊕ commitlint --print-config json  (hard validity gate — §6)
   ↓
prompt       system(rules) + user(budgeted diff) + JSON schema derived from RuleSet
   ↓
generate     POST /api/generate {format: <schema>, stream: false, options:{temperature:0.2}}
   ↓
decode       → commit.Message{Type, Scope, Subject, Body, Breaking, BreakingDesc}
   ↓
apply flags  -b sets Breaking; -f appends "Fixes #123, #456" to the footer
   ↓
validate     RuleSet check
             → deterministic repair (lowercase type, strip trailing period, rewrap body)
             → still failing? one model retry with the violations listed in the prompt
             → still failing? exit 7 and print the message for manual use
   ↓
gate         if commitlint is runnable: pipe the rendered message through
             `commitlint --edit <tmpfile>`; on failure, one repair+retry, then exit 8
   ↓
render       conventional-commit text
   ↓
-n           print and exit 0
-y / no TTY  git commit -F -
else         [y] commit   [e] $EDITOR   [r] regenerate   [q] abort
```

### Nothing staged

`gitia commit` reads only `git diff --staged`. If the staging area is empty it exits 4
with `no staged changes; stage them with git add`. gitia never stages on your behalf,
so it cannot commit files you were not ready to ship.

### Confirmation

Without `-y`, the rendered message is printed and a single-keypress menu offered:

- `y` — commit
- `e` — write to a temp file, open `$EDITOR` (falling back to `$VISUAL`, then `vi`),
  commit the saved result; an empty buffer aborts
- `r` — regenerate, appending the previous message to the prompt with an instruction to
  produce something materially different, and raising temperature by 0.2 per re-roll to
  a ceiling of 0.8. Re-rolls cost about two seconds and are the primary escape hatch
  when the model picks a poor type or a vague subject.
- `q` — abort, exit 130

When stdout is not a TTY, gitia behaves as if `-y` were passed.

## 5. Structured output

Ollama's `format` field accepts a JSON schema and constrains the decoder, not just the
prompt. gitia builds the schema from the resolved `RuleSet`:

```json
{
  "type": "object",
  "properties": {
    "type":    {"type": "string", "enum": ["feat", "fix", "docs", "chore", "..."]},
    "scope":   {"type": "string"},
    "subject": {"type": "string"},
    "body":    {"type": "array", "items": {"type": "string"}},
    "breaking": {"type": "boolean"},
    "breaking_description": {"type": "string"}
  },
  "required": ["type", "subject"]
}
```

Injecting the resolved `type-enum` as a JSON `enum` means a 2.3B model literally cannot
emit `Feat:` or `feature:` — those token sequences are unreachable in the constrained
decode. This is what makes a model this small viable. The remaining failure modes are
semantic (wrong type chosen, vague subject) rather than syntactic, and those are what
`[r]` addresses.

`body` is an array of paragraphs rather than a single string so that rewrapping to
`body-max-line-length` is a deterministic post-process rather than something the model
has to get right.

## 6. Rule precedence

Five layers govern **style**, resolved lowest to highest:

1. Conventional Commits v1.0.0 — baseline
2. `~/.config/gitia/config.toml` — user global
3. Repo-root `AGENTS.md` / `CLAUDE.md` / `GEMINI.md` — all read; `AGENTS.md` wins
   conflicts when several exist
4. Nested `AGENTS.md` covering staged paths — nearest file to the change wins, per the
   agents.md convention
5. CLI flags — explicit user intent

commitlint is **not** a layer in that stack. It is a hard validity gate applied across
all of them. If `type-enum` is `[feat, fix]` and `AGENTS.md` asks for `chore:`, honoring
`AGENTS.md` would produce a commit the repository's own hook rejects. gitia conforms to
commitlint and prints a one-line warning naming the rule that overrode the preference:

```
warning: commitlint type-enum overrode AGENTS.md preference "chore" → "fix"
```

Style preferences commitlint does not constrain pass through untouched.

### commitlint resolution

commitlint configs are usually JavaScript (`commitlint.config.js`, `.commitlintrc.ts`,
or a `commitlint` key in `package.json`) and almost always `extends`
`@commitlint/config-conventional`, which lives in `node_modules`. A Go binary can read
those files but cannot evaluate them.

gitia therefore uses commitlint as its own config oracle: it shells out to
`commitlint --print-config json`, which resolves the entire `extends` chain inside
commitlint's runtime and prints the fully materialized ruleset as JSON. gitia consumes
that JSON into typed Go structs and never needs to understand JavaScript.

Runner detection, by lockfile at the repo root:

| Lockfile | Runner |
| --- | --- |
| `bun.lock` / `bun.lockb` | `bun x commitlint` |
| `pnpm-lock.yaml` | `pnpm exec commitlint` |
| `yarn.lock` | `yarn commitlint` |
| `package-lock.json` | `npx --no-install commitlint` |

`--no-install` on the npx path prevents a surprise network install. `commitlint.runner`
in config overrides detection; `none` disables the integration.

The resolved JSON is cached under the config directory, keyed on a hash of the config
file's path and mtime plus the lockfile's mtime, so the ~1–2 s Node startup is paid once
rather than on every commit.

Detection is skipped entirely when no commitlint config file exists, so non-JS repos pay
nothing.

### AGENTS.md extraction

Discovery walks from the repository root down to the directory of each staged file,
collecting `AGENTS.md` at every level, plus `CLAUDE.md` and `GEMINI.md` at the root only.
Nearest-to-the-change wins.

Because these files are prose, "wins" cannot mean structured merging. It is implemented
as prompt ordering: extracted sections are concatenated least-authoritative first, and
the system prompt states that later sections override earlier ones on conflict. Root
`CLAUDE.md` and `GEMINI.md` are emitted before root `AGENTS.md`, which is emitted before
nested `AGENTS.md` in root-to-leaf order. Each block is labelled with its source path so
the model can attribute a rule, and `--verbose` prints the assembled order.

These files are typically long — build commands, test setup, code style — and mostly
irrelevant to writing a commit message. Feeding all of it to a 2B model wastes context
and distracts it. gitia scans Markdown headings and includes only sections whose heading
text matches `commit`, `git`, `changelog`, `message`, or `versioning` (case-insensitive).
If no heading matches and the file is under `agents.max_bytes` (default 2000), the whole
file is included as a fallback so that repositories writing their conventions in prose
are not silently ignored.

There is deliberately no gitia-specific marker syntax. `AGENTS.md` stays vendor-neutral.

## 7. Diff budgeting

Small models degrade as context grows, and speed degrades with it. gitia always includes
`git diff --staged --stat`, which is cheap and high signal. The full diff is included
when it is under `diff.max_bytes` (default 32768).

Over budget, in order:

1. Paths matching `diff.exclude` (default: `*.lock`, `package-lock.json`,
   `pnpm-lock.yaml`, `go.sum`, `*.min.js`, `dist/**`) drop to stat-only.
2. If still over, the largest remaining files lose their hunk bodies, keeping
   name-status.

The model always sees *that* every file changed even when it cannot see *how*, so it
never invents a scope for a file it did not know about.

Binary files and renames are reported from `--name-status` without hunk content.

## 8. Configuration

Directory resolution: `$GITIA_CONFIG_DIR` → `$XDG_CONFIG_HOME/gitia` → `~/.config/gitia`
on both macOS and Linux. CLI convention beats `~/Library/Application Support` here.
File: `config.toml`. A missing file is not an error; all values have defaults.

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

### Model choice

Default is `gemma4:e2b-it-qat` — 4.3 GB, quantization-aware trained, so it retains more
quality per byte than the 7.2 GB `q4_K_M` build and leaves meaningful headroom on a
16 GB M1. `gemma4:e2b` is first in the fallback chain. Gemma 4 E2B is a Per-Layer
Embedding model: 2.3 B effective parameters, 5.1 B with embeddings, 128 K context.

`fallback` entries are tried in order when `name` is not present locally, before gitia
offers to pull.

### Missing dependencies

gitia never installs anything implicitly.

- Ollama unreachable → exit 5 with the concrete fix (`brew install ollama`,
  `ollama serve`) and a pointer to `gitia doctor`.
- Ollama reachable but the model absent → prompt
  `pull gemma4:e2b-it-qat (~4.3GB)? [y/N]` and stream progress from `/api/pull`. Under
  `-y` or a non-TTY, exit 6 printing the `ollama pull` command to run.

## 9. Errors and exit codes

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

`main` is the only place exit codes are produced. Packages return typed errors that
`cmd/gitia/main.go` maps via `errors.As`.

## 10. Testing

**Table tests** — the precedence matrix (each layer overriding the one below, across the
full rule surface), AGENTS.md heading extraction, commitlint-JSON → `RuleSet` mapping,
message render/validate/repair, diff budgeting under each degradation step, and the
`--fixes` pre-parse.

**git** — real `git` against temporary repositories created per test. git is a
dependency we control the invocation of; mocking it would test the mock.

**Ollama** — `httptest.Server` implementing `/api/tags`, `/api/show`, and
`/api/generate`, including schema-shaped responses and error cases. A separate
`//go:build ollama` suite hits a real local Ollama for smoke coverage; CI skips it.

**commitlint** — a fixture repository per package manager containing a real
`commitlint.config.js` and a vendored `node_modules`, gated behind `//go:build node`.
The pure `--print-config` JSON → `RuleSet` mapping is table-tested from captured
fixtures with no Node required.

**CLI** — `testscript` for end-to-end behavior: exit codes, `--dry-run` output, the
nothing-staged path, `doctor --json`.

**Golden files** for rendered commit messages.

## 11. Distribution

GoReleaser builds darwin and linux for amd64 and arm64 with `CGO_ENABLED=0` and version
metadata via `-ldflags -X`. Release archives contain the binary, `man/gitia.1`, README,
and LICENSE. A Homebrew tap formula installs the binary and the man page.

The man page is generated from the cobra command tree and wrapped in a hand-written
`man/gitia.1.tmpl` supplying NAME, SYNOPSIS, DESCRIPTION, ENVIRONMENT, FILES, EXIT
STATUS, and SEE ALSO — the sections cobra does not emit but `brew.1` has. `make man`
regenerates it; CI fails if the committed page is stale.

`gitia completion bash|zsh|fish` comes from cobra at no cost.

## 12. Open questions

None. Items deferred past MVP are listed under Non-goals.

## References

- Conventional Commits v1.0.0 — https://www.conventionalcommits.org/en/v1.0.0/
- AGENTS.md — https://agents.md/
- commitlint CLI (`--print-config`) — https://commitlint.js.org/reference/cli.html
- commitlint rules — https://commitlint.js.org/reference/rules.html
- Ollama API (`format`, structured outputs) — https://github.com/ollama/ollama/blob/main/docs/api.md
- Gemma 4 E2B — https://huggingface.co/google/gemma-4-E2B
