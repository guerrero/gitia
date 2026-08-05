package prompt

import (
	"encoding/json"

	"github.com/guerrero/gitia/internal/rules"
)

// Schema builds the JSON schema sent in Ollama's format field. Ollama
// constrains the decoder to it, not just the prompt, so injecting the resolved
// type-enum means a 2.3B model literally cannot emit "Feat:" or "feature:" —
// those token sequences are unreachable. The remaining failure modes are
// semantic rather than syntactic, and those are what regeneration addresses.
func Schema(rs rules.RuleSet) json.RawMessage {
	typ := map[string]any{"type": "string"}
	if len(rs.Types) > 0 {
		typ["enum"] = rs.Types
	}

	scope := map[string]any{"type": "string"}
	if len(rs.Scopes) > 0 {
		scope["enum"] = rs.Scopes
	}

	properties := map[string]any{
		"type":    typ,
		"scope":   scope,
		"subject": map[string]any{"type": "string"},
		"breaking": map[string]any{
			"type": "boolean",
		},
		"breaking_description": map[string]any{"type": "string"},
	}

	if rs.IncludeBody {
		// An array of paragraphs, not one string, so rewrapping to
		// body-max-line-length is a deterministic post-process.
		properties["body"] = map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		}
	}

	required := []string{"type", "subject"}
	if rs.ScopeRequired {
		required = append(required, "scope")
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}

	// A schema built from typed Go values cannot fail to marshal.
	out, _ := json.Marshal(schema)
	return out
}
