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

## Releasing

Releases are automated by `.github/workflows/release.yml` and documented here. Pushing a tag publishes
binary assets to a GitHub Release and updates the Homebrew formula in
`guerrero/homebrew-tap` via GoReleaser in CI.

Versioning follows Semantic Versioning. Until 1.0: `feat` bumps the minor
version, `fix` bumps the patch version, breaking changes bump the minor
version.

Checklist:

1. `make test lint man` — the suite is green and `man/gitia.1` is current.
2. Move the `[Unreleased]` section in `CHANGELOG.md` to `[x.y.z] - YYYY-MM-DD`
   and update the compare links at the bottom.
3. Commit (use gitia itself) and push: `git push origin main`.
4. Tag and push: `git tag vX.Y.Z && git push origin vX.Y.Z`. The tag push starts the release workflow.
5. Only if GitHub Actions cannot run, use the local fallback *instead of* pushing a tag:
   `GITHUB_TOKEN=$(gh auth token) make release` — goreleaser builds the
   darwin/linux archives, creates the GitHub Release with notes from the
   commits since the last tag, and pushes `gitia.rb` (at the tap root) to
   `guerrero/homebrew-tap`. `make release` then strips the redundant
   `version` line goreleaser emits, since brew derives the version from the
   URL — this keeps `brew audit` green. The token needs `repo` scope (write
   access to both repositories). Do not run this after a tag push that already
   triggered the workflow.
6. Verify: `brew update && brew upgrade gitia` installs the new version.
