package prompt_test

import (
	"encoding/json"
	"testing"

	"github.com/guerrero/gitia/internal/prompt"
	"github.com/guerrero/gitia/internal/rules"
)

func decodeSchema(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("schema is not valid JSON: %v\n%s", err, raw)
	}
	return m
}

func props(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	p, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no properties object: %+v", schema)
	}
	return p
}

func TestSchemaInjectsTheResolvedTypeEnum(t *testing.T) {
	rs := rules.Conventional()
	rs.Types = []string{"feat", "fix", "chore"}

	schema := decodeSchema(t, prompt.Schema(rs))
	typ, ok := props(t, schema)["type"].(map[string]any)
	if !ok {
		t.Fatal("schema has no type property")
	}

	enum, ok := typ["enum"].([]any)
	if !ok {
		t.Fatalf("type has no enum: %+v", typ)
	}
	if len(enum) != 3 {
		t.Fatalf("enum = %v, want 3 members", enum)
	}
	for i, want := range []string{"feat", "fix", "chore"} {
		if enum[i] != want {
			t.Errorf("enum[%d] = %v, want %q", i, enum[i], want)
		}
	}
}

func TestSchemaSingleTypeWhenForced(t *testing.T) {
	rs := rules.Conventional()
	rs.Types = []string{"docs"}

	schema := decodeSchema(t, prompt.Schema(rs))
	typ := props(t, schema)["type"].(map[string]any)

	if enum := typ["enum"].([]any); len(enum) != 1 || enum[0] != "docs" {
		t.Errorf("enum = %v, want exactly [docs] so the decode cannot choose otherwise", enum)
	}
}

func TestSchemaBodyIsAnArrayOfParagraphs(t *testing.T) {
	schema := decodeSchema(t, prompt.Schema(rules.Conventional()))

	body, ok := props(t, schema)["body"].(map[string]any)
	if !ok {
		t.Fatal("schema has no body property")
	}
	if body["type"] != "array" {
		t.Errorf("body type = %v, want array", body["type"])
	}
	items, ok := body["items"].(map[string]any)
	if !ok || items["type"] != "string" {
		t.Errorf("body items = %+v, want string items", body["items"])
	}
}

func TestSchemaOmitsBodyWhenDisabled(t *testing.T) {
	rs := rules.Conventional()
	rs.IncludeBody = false

	schema := decodeSchema(t, prompt.Schema(rs))
	if _, present := props(t, schema)["body"]; present {
		t.Error("schema has a body property with IncludeBody false")
	}
}

func TestSchemaScopeEnumOnlyWhenConstrained(t *testing.T) {
	rs := rules.Conventional()

	schema := decodeSchema(t, prompt.Schema(rs))
	scope := props(t, schema)["scope"].(map[string]any)
	if _, present := scope["enum"]; present {
		t.Error("scope has an enum when Scopes is empty; any scope must be allowed")
	}

	rs.Scopes = []string{"cli", "git"}
	schema = decodeSchema(t, prompt.Schema(rs))
	scope = props(t, schema)["scope"].(map[string]any)
	if enum, ok := scope["enum"].([]any); !ok || len(enum) != 2 {
		t.Errorf("scope enum = %v, want the two allowed scopes", scope["enum"])
	}
}

func TestSchemaRequired(t *testing.T) {
	rs := rules.Conventional()

	schema := decodeSchema(t, prompt.Schema(rs))
	req := schema["required"].([]any)
	if len(req) != 2 || req[0] != "type" || req[1] != "subject" {
		t.Errorf("required = %v, want [type subject]", req)
	}

	rs.ScopeRequired = true
	schema = decodeSchema(t, prompt.Schema(rs))
	req = schema["required"].([]any)
	if len(req) != 3 {
		t.Errorf("required = %v, want scope added when ScopeRequired", req)
	}
}
