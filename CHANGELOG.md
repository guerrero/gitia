# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-08-05

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

[Unreleased]: https://github.com/guerrero/gitia/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/guerrero/gitia/releases/tag/v0.1.0
