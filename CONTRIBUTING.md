# Contributing

## Setup

```bash
git clone https://github.com/guerrero/gitia
cd gitia
make build test lint
```

Go 1.26 is required. Nothing else is: the test suite runs without Ollama, without
Node, and without network access.

## Running the tests without a local Ollama

The default suite never touches the network. `internal/ollama` is tested against
an `httptest.Server`, and `internal/rules` maps captured
`commitlint --print-config json` fixtures with no Node involved.

Two build-tagged suites exercise the real integrations and are skipped by CI:

```bash
go test -tags ollama ./internal/ollama/   # needs `ollama serve` and the default model
go test -tags node   ./internal/rules/    # needs Node and a vendored node_modules
```

Both skip themselves rather than failing when their dependency is absent.

## Regenerating the man page

`man/gitia.1` is committed and CI fails when it drifts from the command tree:

```bash
make man
```

## Architecture

`internal/rules` is the only package that knows about rule precedence.
`internal/commit` validates against a `RuleSet` it is handed and knows nothing
about where the rules came from; `internal/prompt` derives the Ollama JSON schema
from the same `RuleSet`. Keep that boundary — it is what makes the precedence
matrix testable as a pure table test with no git, no filesystem, and no Ollama.

`main` is the only place exit codes are produced. Packages return errors wrapped
with `exitcode.Wrap`, and `cmd/gitia/main.go` maps them via `errors.As`.

## Commit messages

See `AGENTS.md`. Use gitia itself.
