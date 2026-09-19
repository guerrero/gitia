# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-09-19

### Added

- `commit.header_ideal_length` configures the soft 50–55 character header
  target the prompt suggests; it is guidance only, never enforced.

### Changed

- Default model is now `lfm2.5:8b`, with `gemma4:e2b-it-qat` as fallback.
- The default body line width is now 72 characters (was 100), matching the
  header limit. Both remain configurable.

## [0.2.0] - 2026-08-06

### Added

- `gitia commit` generates a Conventional Commits message from the staged diff
  using a model running locally under Ollama. Nothing leaves the machine.
- Structured output: the JSON schema sent to Ollama is derived from the resolved
  rules, so the constrained decode cannot produce an illegal commit type.
- Rule precedence across Conventional Commits defaults, `~/.config/gitia/config.toml`,
  `AGENTS.md`/`CLAUDE.md`/`GEMINI.md`, and CLI flags.
- commitlint as a hard validity gate, resolved through
  `commitlint --print-config json` and cached on the config and lockfile mtimes.
- Diff budgeting: excluded paths drop to stat-only first, then the largest
  remaining files, while every changed path stays listed.
- `gitia doctor` with ten checks and a `--json` mode.
- A confirmation menu with commit, edit, regenerate, and abort.
- `man gitia` and bash, zsh, and fish completions.
- `make release`: a guarded release target that publishes assets and updates
  the Homebrew tap formula. The checklist lives in CONTRIBUTING.md.

### Changed

- Polished `gitia doctor` output, the editor flow, prompt handling, and commit
  feedback.

### Fixed

- Usage errors exit with status 2.
- Prompts abort when the command context is canceled.
- Reroll keeps its context when a validation retry is needed.

### Removed

- GitHub Actions workflows; releases are manual now.

[Unreleased]: https://github.com/guerrero/gitia/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/guerrero/gitia/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/guerrero/gitia/releases/tag/v0.2.0
