//go:build ollama

// Package ollama_test's live suite hits a real local Ollama server. CI skips
// it; run it with: go test -tags ollama ./internal/ollama/
package ollama_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/guerrero/gitia/internal/ollama"
)

const liveModel = "gemma4:e2b-it-qat"

func liveClient(t *testing.T) *ollama.Client {
	t.Helper()
	c := ollama.New("http://localhost:11434", 120*time.Second)

	if _, err := c.Tags(t.Context()); err != nil {
		t.Skipf("no local Ollama: %v", err)
	}
	has, err := c.Has(t.Context(), liveModel)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Skipf("model %s not present locally; run: ollama pull %s", liveModel, liveModel)
	}
	return c
}

func TestLiveGenerateHonorsTheSchemaEnum(t *testing.T) {
	c := liveClient(t)

	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"type": {"type": "string", "enum": ["feat", "fix"]},
			"subject": {"type": "string"}
		},
		"required": ["type", "subject"]
	}`)

	resp, err := c.Generate(t.Context(), ollama.GenerateRequest{
		Model:   liveModel,
		System:  "You write conventional commit messages. Reply with JSON only.",
		Prompt:  "diff --git a/README.md b/README.md\n+A new paragraph about installation.\n",
		Format:  schema,
		Options: ollama.Options{Temperature: 0.2, NumCtx: 8192},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var got struct {
		Type    string `json:"type"`
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal([]byte(resp.Response), &got); err != nil {
		t.Fatalf("model returned non-JSON despite the schema: %q", resp.Response)
	}
	if got.Type != "feat" && got.Type != "fix" {
		t.Errorf("type = %q; the schema enum makes any other value unreachable", got.Type)
	}
	if got.Subject == "" {
		t.Error("subject is empty")
	}
}
