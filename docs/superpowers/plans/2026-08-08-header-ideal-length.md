# Header Ideal Length and Body Default Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make 72 the default max width for both header and body, and add a soft, prompt-only 50–55 character ideal header range, both overridable via config.

**Architecture:** The ideal range lives in the `RuleSet` style contract (like the existing prompt-only `Language` field): the `Conventional()` baseline defines `HeaderIdealMin = 50` / `HeaderIdealMax = 55`, `[commit]` config overlays them via `header_ideal_length`, and `prompt.System` renders one extra guidance line. Validation and repair are untouched — the ideal range is never enforced.

**Tech Stack:** Go, BurntSushi/toml config, stdlib testing. No new dependencies.

## Global Constraints

- `HeaderMaxLength` default stays **72** (hard limit, unchanged behavior).
- `BodyMaxLineLength` default changes from 100 to **72**; still overridable via `commit.body_max_line_length`.
- Ideal range defaults to **50–55**, overridable via `commit.header_ideal_length = [50, 55]`.
- The ideal range is **prompt guidance only**: `internal/commit/validate.go` and `internal/commit/repair.go` are NOT touched. No warnings, no CLI flags, no new precedence layers.
- `Config.Apply` semantics for `HeaderIdealLength`: empty slice or 1 element = "not configured" (leave layer below untouched); ≥ 2 elements override with `v[0]` = min, `v[1]` = max, extras ignored; `[0, 0]` explicitly disables.
- Prompt emission rule: emit only when `min > 0 && max > 0 && min <= max`; clamp `max` down to `HeaderMaxLength` when `HeaderMaxLength > 0 && max > HeaderMaxLength`; after clamping, omit if `min > max`.
- Prompt line text (exact, en-dash included): `- Prefer a header of 50–55 characters; the limit above is a ceiling, not a target.`
- `internal/rules/commitlint_test.go` fixtures keep `100` — they mirror `@commitlint/config-conventional`, not gitia's defaults.
- `man/gitia.1` does NOT change: `man/gitia.1.tmpl` documents no length defaults (verified).
- Commit conventions (repo AGENTS.md): Conventional Commits, imperative lowercase subject, no trailing period, header ≤ 72 chars. Scope by package: `rules`, `prompt`; no scope for repo-wide `docs`.
- Code style: standard Go with gofumpt.

---

### Task 1: Baseline and RuleSet fields

**Files:**
- Modify: `internal/rules/ruleset.go` (add two fields after `BodyMaxLineLength`)
- Modify: `internal/rules/conventional.go` (defaults)
- Test: `internal/rules/conventional_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `rules.RuleSet.HeaderIdealMin int` — 0 means "no guidance".
  - `rules.RuleSet.HeaderIdealMax int` — 0 means "no guidance".
  - `rules.Conventional()` now returns `BodyMaxLineLength = 72`, `HeaderIdealMin = 50`, `HeaderIdealMax = 55`.

- [ ] **Step 1: Update the test to the new expectations**

In `internal/rules/conventional_test.go`, change the body expectation and add the ideal-range assertions:

```go
	if got, want := rs.BodyMaxLineLength, 72; got != want {
		t.Errorf("BodyMaxLineLength = %d, want %d", got, want)
	}
	if got, want := rs.HeaderIdealMin, 50; got != want {
		t.Errorf("HeaderIdealMin = %d, want %d", got, want)
	}
	if got, want := rs.HeaderIdealMax, 55; got != want {
		t.Errorf("HeaderIdealMax = %d, want %d", got, want)
	}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/rules/ -run TestConventionalBaseline -v`
Expected: FAIL — `BodyMaxLineLength = 100, want 72` and undefined-field compile errors for `HeaderIdealMin`/`HeaderIdealMax`.

- [ ] **Step 3: Add the fields to RuleSet**

In `internal/rules/ruleset.go`, after the `BodyMaxLineLength` field:

```go
	// HeaderIdealMin is the lower bound of the soft header width the model is
	// asked to aim for. It is prompt guidance only: Validate never enforces
	// it. Zero disables the guidance.
	HeaderIdealMin int
	// HeaderIdealMax is the upper bound of the soft header width. It is never
	// enforced; zero disables the guidance.
	HeaderIdealMax int
```

- [ ] **Step 4: Update the baseline**

In `internal/rules/conventional.go`, change `BodyMaxLineLength: 100` to `BodyMaxLineLength: 72`, and after the `HeaderMaxLength` line add:

```go
		HeaderIdealMin:      50,
		HeaderIdealMax:      55,
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/rules/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/rules/ruleset.go internal/rules/conventional.go internal/rules/conventional_test.go
git commit -m "feat(rules): set body default to 72 and add ideal header range"
```

---

### Task 2: Config schema and Apply overlay

**Files:**
- Modify: `internal/rules/config.go`
- Test: `internal/rules/config_test.go`

**Interfaces:**
- Consumes: `rules.RuleSet.HeaderIdealMin`/`HeaderIdealMax` from Task 1.
- Produces:
  - `rules.CommitConfig.HeaderIdealLength []int` with TOML key `header_ideal_length`.
  - `rules.DefaultConfig()` seeds `Commit.HeaderIdealLength = [50, 55]` and `BodyMaxLineLength = 72` (already copied from the baseline — verify).
  - `Config.Apply` overlays the ideal fields with the semantics in Global Constraints.

- [ ] **Step 1: Write the failing tests**

In `internal/rules/config_test.go`:

a) In `TestDefaultConfig`, after the `cfg.Commit.Body` assertion:

```go
	if got := cfg.Commit.HeaderIdealLength; len(got) != 2 || got[0] != 50 || got[1] != 55 {
		t.Errorf("Commit.HeaderIdealLength = %v, want [50 55]", got)
	}
```

b) In `TestLoadConfigPartialFileKeepsOtherDefaults`, change `want 100` to `want 72`:

```go
	if cfg.Commit.BodyMaxLineLength != 72 {
		t.Errorf("Commit.BodyMaxLineLength = %d; an absent key must keep its default", cfg.Commit.BodyMaxLineLength)
	}
```

c) New test, after `TestLoadConfigPartialFileKeepsOtherDefaults`:

```go
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
```

d) In `TestConfigApply`, add after the `cfg.Commit.BodyMaxLineLength = 72` line:

```go
	cfg.Commit.HeaderIdealLength = []int{40, 60}
```

and after the `BodyMaxLineLength` assertion:

```go
	if rs.HeaderIdealMin != 40 || rs.HeaderIdealMax != 60 {
		t.Errorf("HeaderIdeal = [%d %d], want [40 60]", rs.HeaderIdealMin, rs.HeaderIdealMax)
	}
```

e) New test, after `TestConfigApplyEmptyTypesKeepsTheBaseline`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/rules/`
Expected: FAIL — `Commit.HeaderIdealLength` does not exist (compile error); `BodyMaxLineLength` default assertions report 100/72 mismatches.

- [ ] **Step 3: Implement the config field, default, and overlay**

In `internal/rules/config.go`:

a) In `CommitConfig`, after `HeaderMaxLength`:

```go
	HeaderIdealLength  []int  `toml:"header_ideal_length"`
```

(Align the struct tags with gofumpt when saving — the struct uses aligned tags.)

b) In `DefaultConfig`, after the `HeaderMaxLength: base.HeaderMaxLength,` line:

```go
			HeaderIdealLength:  []int{base.HeaderIdealMin, base.HeaderIdealMax},
```

`BodyMaxLineLength: base.BodyMaxLineLength` already copies the new 72 — no change needed there.

c) In `Config.Apply`, after the `HeaderMaxLength` overlay block:

```go
	if len(c.Commit.HeaderIdealLength) >= 2 {
		out.HeaderIdealMin = c.Commit.HeaderIdealLength[0]
		out.HeaderIdealMax = c.Commit.HeaderIdealLength[1]
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/rules/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/rules/config.go internal/rules/config_test.go
git commit -m "feat(rules): make header_ideal_length configurable"
```

---

### Task 3: Prompt guidance

**Files:**
- Modify: `internal/prompt/prompt.go`
- Test: `internal/prompt/prompt_test.go`

**Interfaces:**
- Consumes: `rules.RuleSet.HeaderIdealMin`/`HeaderIdealMax` (Task 1) with the overlay from Task 2.
- Produces: the guidance line inside `prompt.System(rs rules.RuleSet, docs []rules.AgentDoc) string`, rendered immediately after the hard-limit line.

- [ ] **Step 1: Write the failing tests**

In `internal/prompt/prompt_test.go`, add after `TestSystemStatesTheRules`:

```go
func TestSystemGuidesTowardTheIdealHeaderWidth(t *testing.T) {
	got := prompt.System(rules.Conventional(), nil)

	want := "- Prefer a header of 50–55 characters; the limit above is a ceiling, not a target."
	if !strings.Contains(got, want) {
		t.Errorf("System() is missing the ideal-width guidance\n%s", got)
	}
}

func TestSystemOmitsIdealHeaderWidthWhenDisabled(t *testing.T) {
	rs := rules.Conventional()
	rs.HeaderIdealMin = 0
	rs.HeaderIdealMax = 0

	got := prompt.System(rs, nil)
	if strings.Contains(got, "Prefer a header") {
		t.Errorf("System() must not emit the guidance when the range is disabled\n%s", got)
	}
}

func TestSystemOmitsInvertedIdealRange(t *testing.T) {
	rs := rules.Conventional()
	rs.HeaderIdealMin = 60
	rs.HeaderIdealMax = 50

	got := prompt.System(rs, nil)
	if strings.Contains(got, "Prefer a header") {
		t.Errorf("System() must not emit an inverted range\n%s", got)
	}
}

func TestSystemClampsIdealRangeToTheHardLimit(t *testing.T) {
	rs := rules.Conventional()
	rs.HeaderIdealMax = 80

	got := prompt.System(rs, nil)
	if !strings.Contains(got, "Prefer a header of 50–72 characters") {
		t.Errorf("System() must clamp the ideal max to the hard limit\n%s", got)
	}

	rs = rules.Conventional()
	rs.HeaderIdealMin = 80
	rs.HeaderIdealMax = 90

	got = prompt.System(rs, nil)
	if strings.Contains(got, "Prefer a header") {
		t.Errorf("System() must omit a range clamped past the hard limit\n%s", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/prompt/ -run 'TestSystem(Guides|Omits|Clamps)' -v`
Expected: FAIL — the guidance line is absent.

- [ ] **Step 3: Implement the guidance line**

In `internal/prompt/prompt.go`, immediately after the `HeaderMaxLength` block in `System`:

```go
	if rs.HeaderIdealMin > 0 && rs.HeaderIdealMax > 0 && rs.HeaderIdealMin <= rs.HeaderIdealMax {
		max := rs.HeaderIdealMax
		if rs.HeaderMaxLength > 0 && max > rs.HeaderMaxLength {
			max = rs.HeaderMaxLength
		}
		if rs.HeaderIdealMin <= max {
			fmt.Fprintf(&b, "- Prefer a header of %d–%d characters; the limit above is a ceiling, not a target.\n",
				rs.HeaderIdealMin, max)
		}
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/prompt/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/prompt/prompt.go internal/prompt/prompt_test.go
git commit -m "feat(prompt): guide the model toward the ideal header width"
```

---

### Task 4: Documentation

**Files:**
- Modify: `README.md` (example config block, around line 77)
- Modify: `CHANGELOG.md` (`[Unreleased]` section)
- Modify: `AGENTS.md` (Commit conventions section)
- Verify: `man/gitia.1` needs NO regeneration (template documents no length defaults).

**Interfaces:**
- Consumes: the behavior from Tasks 1–3.
- Produces: nothing consumed by code.

- [ ] **Step 1: Update the README example config**

In `README.md`, in the `[commit]` block:

```
header_max_length    = 72   # whole "type(scope): subject" line, as commitlint counts it
body_max_line_length = 100
```

becomes:

```
header_max_length    = 72   # whole "type(scope): subject" line, as commitlint counts it
header_ideal_length  = [50, 55]  # soft guidance for the model, never enforced
body_max_line_length = 72
```

(Align the comment column with the surrounding lines when editing — the block uses aligned `=` and comments.)

- [ ] **Step 2: Update the changelog**

In `CHANGELOG.md`, fill the empty `[Unreleased]` section:

```markdown
## [Unreleased]

### Added

- `commit.header_ideal_length` configures the soft 50–55 character header
  target the prompt suggests; it is guidance only, never enforced.

### Changed

- The default body line width is now 72 characters (was 100), matching the
  header limit. Both remain configurable.
```

- [ ] **Step 3: Dogfood the guidance in AGENTS.md**

In `AGENTS.md`, in the Commit conventions section, change:

"The subject is imperative, lower case, and has no trailing period. The header stays within 72 characters."

to:

"The subject is imperative, lower case, and has no trailing period. The header stays within 72 characters and ideally within 50–55."

- [ ] **Step 4: Run the full suite and lint**

Run: `make test`
Expected: PASS.
Run: `make lint`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add README.md CHANGELOG.md AGENTS.md
git commit -m "docs: document header ideal length and body default"
```

---

## Self-Review Notes

- **Spec coverage:** RuleSet fields + baseline (Task 1); config schema, default, Apply overlay with all four edge semantics (Task 2); prompt line with clamp/omit rules and exact wording (Task 3); README, CHANGELOG, AGENTS.md, man verified (Task 4). `validate.go`/`repair.go`/commitlint fixtures untouched by design.
- **Placeholder scan:** every step carries real code or an exact edit.
- **Type consistency:** `HeaderIdealMin`/`HeaderIdealMax` (int) and `CommitConfig.HeaderIdealLength` (`[]int`, TOML `header_ideal_length`) are spelled identically across all four tasks.
