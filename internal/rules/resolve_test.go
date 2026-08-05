package rules_test

import (
	"strings"
	"testing"

	"github.com/guerrero/gitia/internal/rules"
)

func TestResolveBaselineOnly(t *testing.T) {
	got := rules.Resolve(rules.Layers{Config: rules.DefaultConfig()})

	base := rules.Conventional()
	if len(got.Rules.Types) != len(base.Types) {
		t.Errorf("Types = %v, want the conventional baseline", got.Rules.Types)
	}
	if got.Rules.HeaderMaxLength != 72 {
		t.Errorf("HeaderMaxLength = %d, want 72", got.Rules.HeaderMaxLength)
	}
	if len(got.Overrides) != 0 {
		t.Errorf("Overrides = %+v, want none", got.Overrides)
	}
}

func TestResolvePrecedenceMatrix(t *testing.T) {
	conventional := []string{"feat", "fix", "docs", "style", "refactor", "perf",
		"test", "build", "ci", "chore", "revert"}

	restrictive := rules.Constraints{}
	types := []string{"feat", "fix", "chore"}
	restrictive.Types = &types
	limit := 50
	restrictive.HeaderMaxLength = &limit

	userConfig := rules.DefaultConfig()
	userConfig.Commit.Types = []string{"feat", "fix", "chore", "docs"}
	userConfig.Commit.HeaderMaxLength = 60

	tests := []struct {
		name            string
		layers          rules.Layers
		wantTypes       []string
		wantHeaderMax   int
		wantOverride    string
		wantNoOverrides bool
	}{
		{
			name:            "layer 1 only: the baseline",
			layers:          rules.Layers{Config: rules.DefaultConfig()},
			wantTypes:       conventional,
			wantHeaderMax:   72,
			wantNoOverrides: true,
		},
		{
			name:            "layer 2 beats layer 1: user config",
			layers:          rules.Layers{Config: userConfig},
			wantTypes:       []string{"feat", "fix", "chore", "docs"},
			wantHeaderMax:   60,
			wantNoOverrides: true,
		},
		{
			name: "layer 5 beats layer 2: --type narrows to one",
			layers: rules.Layers{
				Config: userConfig,
				Flags:  rules.FlagOverrides{Type: "docs"},
			},
			wantTypes:       []string{"docs"},
			wantHeaderMax:   60,
			wantNoOverrides: true,
		},
		{
			name: "commitlint gates every layer, including flags",
			layers: rules.Layers{
				Config:      userConfig,
				Flags:       rules.FlagOverrides{Type: "docs"},
				Constraints: &restrictive,
			},
			wantTypes:     []string{"feat", "fix", "chore"},
			wantHeaderMax: 50,
			wantOverride:  "type-enum",
		},
		{
			name: "commitlint narrows the user config",
			layers: rules.Layers{
				Config:      userConfig,
				Constraints: &restrictive,
			},
			wantTypes:     []string{"feat", "fix", "chore"},
			wantHeaderMax: 50,
			wantOverride:  "type-enum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rules.Resolve(tt.layers)

			if len(got.Rules.Types) != len(tt.wantTypes) {
				t.Fatalf("Types = %v, want %v", got.Rules.Types, tt.wantTypes)
			}
			for i := range tt.wantTypes {
				if got.Rules.Types[i] != tt.wantTypes[i] {
					t.Errorf("Types[%d] = %q, want %q", i, got.Rules.Types[i], tt.wantTypes[i])
				}
			}
			if got.Rules.HeaderMaxLength != tt.wantHeaderMax {
				t.Errorf("HeaderMaxLength = %d, want %d", got.Rules.HeaderMaxLength, tt.wantHeaderMax)
			}

			if tt.wantNoOverrides && len(got.Overrides) != 0 {
				t.Errorf("Overrides = %+v, want none", got.Overrides)
			}
			if tt.wantOverride != "" {
				var found bool
				for _, o := range got.Overrides {
					if o.Rule == tt.wantOverride {
						found = true
					}
				}
				if !found {
					t.Errorf("Overrides = %+v, want a %q entry", got.Overrides, tt.wantOverride)
				}
			}
		})
	}
}

func TestResolveScopeFlagNarrowsAndRequires(t *testing.T) {
	got := rules.Resolve(rules.Layers{
		Config: rules.DefaultConfig(),
		Flags:  rules.FlagOverrides{Scope: "cli"},
	})

	if len(got.Rules.Scopes) != 1 || got.Rules.Scopes[0] != "cli" {
		t.Errorf("Scopes = %v, want [cli]", got.Rules.Scopes)
	}
	if !got.Rules.ScopeRequired {
		t.Error("ScopeRequired = false; an explicit --scope must require it")
	}
}

func TestResolveNoBodyFlag(t *testing.T) {
	got := rules.Resolve(rules.Layers{
		Config: rules.DefaultConfig(),
		Flags:  rules.FlagOverrides{NoBody: true},
	})
	if got.Rules.IncludeBody {
		t.Error("IncludeBody = true, want false with --no-body")
	}
}

func TestResolveCarriesAgentDocsInOrder(t *testing.T) {
	docs := []rules.AgentDoc{
		{Path: "CLAUDE.md", Content: "a"},
		{Path: "AGENTS.md", Content: "b"},
		{Path: "pkg/AGENTS.md", Content: "c"},
	}

	got := rules.Resolve(rules.Layers{Config: rules.DefaultConfig(), AgentDocs: docs})

	if len(got.AgentDocs) != 3 {
		t.Fatalf("AgentDocs = %+v, want 3", got.AgentDocs)
	}
	for i := range docs {
		if got.AgentDocs[i].Path != docs[i].Path {
			t.Errorf("AgentDocs[%d].Path = %q, want %q", i, got.AgentDocs[i].Path, docs[i].Path)
		}
	}
}

func TestWarningsNameTheRuleThatWon(t *testing.T) {
	types := []string{"feat", "fix"}
	c := rules.Constraints{Types: &types}

	cfg := rules.DefaultConfig()
	cfg.Commit.Types = []string{"chore", "feat", "fix"}

	got := rules.Resolve(rules.Layers{Config: cfg, Constraints: &c})
	warnings := got.Warnings()

	if len(warnings) != 1 {
		t.Fatalf("Warnings() = %v, want exactly one", warnings)
	}
	w := warnings[0]
	for _, want := range []string{"warning:", "commitlint", "type-enum", "chore", "→"} {
		if !strings.Contains(w, want) {
			t.Errorf("warning %q is missing %q", w, want)
		}
	}
}

func TestWarningsEmptyWhenNothingWasOverridden(t *testing.T) {
	got := rules.Resolve(rules.Layers{Config: rules.DefaultConfig()})
	if len(got.Warnings()) != 0 {
		t.Errorf("Warnings() = %v, want none", got.Warnings())
	}
}
