# AGENTS.md

Conventions for this repository. gitia reads this file when generating its own
commit messages, so it is dogfooded on every commit here.

## Build and test

- `make build` — build the binary with version metadata.
- `make test` — run the full suite.
- `make lint` — golangci-lint.
- `make man` — regenerate `man/gitia.1`. CI fails if the committed page is stale.
- `go test -tags ollama ./internal/ollama/` — the live smoke suite; needs a
  local Ollama and the default model.
- `go test -tags node ./internal/rules/` — the commitlint fixture suite; needs
  Node and a vendored `node_modules`.

## Commit conventions

Conventional Commits v1.0.0. The subject is imperative, lower case, and has no
trailing period. The header stays within 72 characters and ideally within 50–55.

Scope by package, without the `internal/` prefix: `cli`, `commit`, `git`,
`ollama`, `prompt`, `rules`, `ui`. Use no scope for changes that span the whole
repository.

Allowed types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`,
`build`, `ci`, `chore`, `revert`.

Write a body when the change is not self-evident from the subject. Explain why,
not what — the diff already says what. Omit the body for mechanical changes.

## Code style

Standard Go with gofumpt. Package comments on every package. Comments explain
why a decision was made, not what the line does. `internal/rules` is the only
package that knows about precedence; keep it that way.

## Release

Releases are tag-driven; the checklist lives in CONTRIBUTING.md. Version numbers
follow Semantic Versioning (pre-1.0: `feat` → minor, `fix` → patch).

The changelog is curated by hand in Keep a Changelog format: every release moves
the `[Unreleased]` section to a dated version heading. Pushing tag `vX.Y.Z`
from `main` triggers the `release` workflow, which publishes assets and updates
the Homebrew tap formula.
