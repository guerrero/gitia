# Header ideal length and body line default: design

Date: 2026-08-08

## Problem

gitia's defaults for commit message widths come from two different traditions:

- `HeaderMaxLength = 72` — the classic git convention (80-column terminals, the
  width `git commit`'s own template wraps to). Conventional Commits v1.0.0
  defines no length limit at all.
- `BodyMaxLineLength = 100` — inherited from `@commitlint/config-conventional`.

Neither is documented in the code. The user wants:

1. `BodyMaxLineLength` default changed from 100 to **72**, so header and body
   share one hard limit. Still overridable via `commit.body_max_line_length`.
2. `HeaderMaxLength` default stays **72** (hard limit, unchanged behavior).
3. A new **soft** criterion: an ideal header width of 50–55 characters. It is
   prompt guidance only — never validated, never blocks a commit, never
   repaired. Overridable via config.

## Decisions

- **Prompt-guidance only** (no warning, no validation): the model is told to
  aim for the ideal range; the hard limit of 72 is the only enforced rule.
- **The ideal range lives in the `RuleSet`** (option 1 of the brainstorm):
  the `RuleSet` is the style contract and already carries non-enforced,
  prompt-only fields (`Language`, `BodyLeadingBlank`, `FooterLeadingBlank`).
  The prompt is a pure function of the `RuleSet`; no new plumbing.
- **Configurable as a range** `commit.header_ideal_length = [50, 55]`.

## Data model

- `rules.RuleSet` gains:
  - `HeaderIdealMin int` (0 = no guidance)
  - `HeaderIdealMax int` (0 = no guidance)
- `rules.Conventional()` baseline:
  - `BodyMaxLineLength`: 100 → **72**
  - `HeaderIdealMin = 50`, `HeaderIdealMax = 55` (new defaults)
  - `HeaderMaxLength` stays 72.
- `rules.CommitConfig` gains `HeaderIdealLength []int` with TOML key
  `header_ideal_length`; `DefaultConfig` seeds it from the baseline with
  `[50, 55]`.
- `Config.Apply` (same pattern as `Types`):
  - empty slice or 1 element = "not configured", leave the layer below
    untouched;
  - ≥ 2 elements: min = `v[0]`, max = `v[1]`, extras ignored;
  - `[0, 0]` explicitly disables the guidance.
- commitlint and AGENTS.md layers do not touch the ideal range: no commitlint
  rule maps to it, and AGENTS.md prose already reaches the prompt through its
  own channel.

## Prompt behavior

In `prompt.System`, immediately after the hard-limit line
(`- The whole "type(scope): subject" line must be at most 72 characters.`),
emit, when valid:

```
- Prefer a header of 50–55 characters; the limit above is a ceiling, not a target.
```

Validity rules before emitting:

- `min <= 0` or `max <= 0` → omit (disabled).
- `min > max` → omit (inverted range).
- If `HeaderMaxLength > 0` and `max > HeaderMaxLength` → clamp `max` down to
  the hard limit (e.g. `[50, 80]` with limit 72 renders as "50–72"); if the
  clamped range is inverted (`min > max`) → omit (e.g. `[80, 90]` with limit
  72 emits nothing).

The hard-limit line is always emitted, exactly as today. `Retry` and `Reroll`
need no changes: the system prompt is re-sent in full on every attempt.

## Files touched

- `internal/rules/ruleset.go` — two new fields (and `Clone` copies them by
  value, no change needed).
- `internal/rules/conventional.go` — new defaults; body 100 → 72.
- `internal/rules/config.go` — new `CommitConfig` field, default, `Apply`
  overlay.
- `internal/prompt/prompt.go` — the guidance line + clamp/omit logic.
- Tests:
  - `internal/rules/conventional_test.go` — body expects 72; ideal fields.
  - `internal/rules/config_test.go` — body default 72; parse `[50, 55]`;
    overlay cases (≥ 2 overrides, `[]`/`[55]` do not touch, `[0, 0]`
    disables).
  - `internal/prompt/prompt_test.go` (or existing prompt tests) — line
    emitted with values; omitted for `[0, 0]` and inverted; clamped when the
    hard limit is lower.
- `README.md` — example config: `body_max_line_length = 72`,
  `header_ideal_length = [50, 55]`.
- `man/gitia.1` — regenerate via `make man` if the template documents these
  values (verify during implementation).
- `CHANGELOG.md` — `[Unreleased]` entry (feat: `header_ideal_length`; default
  body width now 72).
- `AGENTS.md` — dogfooding: add "aim for a 50–55 character header" next to
  the existing 72-character rule.

## Explicitly out of scope

- No changes to `internal/commit/validate.go` or `repair.go`: the ideal range
  is never enforced or repaired.
- `internal/rules/commitlint_test.go` fixtures keep `100`: they mirror
  `@commitlint/config-conventional`, not gitia's defaults.
- No new precedence layers, no CLI flags.
