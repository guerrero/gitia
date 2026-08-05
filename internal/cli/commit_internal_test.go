package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/guerrero/gitia/internal/commit"
	"github.com/guerrero/gitia/internal/exitcode"
	"github.com/guerrero/gitia/internal/ollama"
	"github.com/guerrero/gitia/internal/rules"
)

// generateStub serves /api/generate, recording every prompt and replying with
// the next canned response, so tests can drive generateOnce over a fake
// Ollama exactly like the real client does.
type generateStub struct {
	t         *testing.T
	mu        sync.Mutex
	responses []string
	prompts   []string
}

func newGenerateStub(t *testing.T, responses ...string) *generateStub {
	return &generateStub{t: t, responses: responses}
}

func (s *generateStub) handler(w http.ResponseWriter, r *http.Request) {
	var req ollama.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.t.Errorf("decode request: %v", err)
		return
	}

	s.mu.Lock()
	s.prompts = append(s.prompts, req.Prompt)
	next := s.responses[len(s.prompts)-1]
	s.mu.Unlock()

	if err := json.NewEncoder(w).Encode(ollama.GenerateResponse{Response: next}); err != nil {
		s.t.Errorf("encode response: %v", err)
	}
}

func (s *generateStub) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(s.handler))
}

func (s *generateStub) lastPrompt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prompts[len(s.prompts)-1]
}

func (s *generateStub) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.prompts)
}

// genArgs builds the generateOnce arguments a real run would produce: the
// conventional baseline rules and the default config.
func genArgs(rs rules.RuleSet) generateArgs {
	return generateArgs{
		model:  "gemma4:e2b-it-qat",
		system: "system prompt",
		user:   "the staged diff",
		schema: json.RawMessage(`{}`),
		cfg:    rules.DefaultConfig(),
		rs:     rs,
	}
}

// TestGenerateOnceRepairsAndRetriesWithTheViolations drives the
// validate->repair->retry path: a first answer that violates the rules after
// repair must trigger a second call whose prompt names the violation, and the
// repaired second answer wins.
func TestGenerateOnceRepairsAndRetriesWithTheViolations(t *testing.T) {
	stub := newGenerateStub(t,
		`{"type":"feature","subject":"x"}`, // unknown type: repair cannot fix it
		`{"type":"feat","subject":"add a thing","body":["because"]}`,
	)
	srv := stub.server()
	defer srv.Close()

	msg, err := generateOnce(t.Context(), io.Discard, ollama.New(srv.URL, 0), genArgs(rules.Conventional()))
	if err != nil {
		t.Fatalf("generateOnce: %v", err)
	}
	if msg.Type != "feat" || msg.Subject != "add a thing" {
		t.Errorf("msg = %+v, want the second answer", msg)
	}
	if got := stub.calls(); got != 2 {
		t.Fatalf("generateOnce made %d calls, want 2", got)
	}
	if !strings.Contains(stub.lastPrompt(), "type-enum") {
		t.Errorf("retry prompt = %q, want it to name the type-enum violation", stub.lastPrompt())
	}
}

// TestGenerateOnceRerollContextSurvivesTheRetry guards the retry prompt
// against dropping the reroll instruction: a regenerated message that still
// violates must be retried with the reroll context intact.
func TestGenerateOnceRerollContextSurvivesTheRetry(t *testing.T) {
	stub := newGenerateStub(t,
		`{"type":"feature","subject":"x"}`,
		`{"type":"feat","subject":"a valid subject"}`,
	)
	srv := stub.server()
	defer srv.Close()

	a := genArgs(rules.Conventional())
	a.previous = &commit.Message{Type: "fix", Subject: "the old angle"}

	if _, err := generateOnce(t.Context(), io.Discard, ollama.New(srv.URL, 0), a); err != nil {
		t.Fatalf("generateOnce: %v", err)
	}
	prompt := stub.lastPrompt()
	if !strings.Contains(prompt, "Produce a materially different message") {
		t.Errorf("retry prompt = %q; the reroll instruction must survive the validation retry", prompt)
	}
	if !strings.Contains(prompt, "type-enum") {
		t.Errorf("retry prompt = %q, want it to name the type-enum violation", prompt)
	}
}

// TestGenerateOnceFailsWithExitCode7 after two invalid answers.
func TestGenerateOnceFailsWithExitCode7(t *testing.T) {
	stub := newGenerateStub(t,
		`{"type":"feature","subject":"x"}`,
		`{"type":"feature","subject":"x"}`,
	)
	srv := stub.server()
	defer srv.Close()

	_, err := generateOnce(t.Context(), io.Discard, ollama.New(srv.URL, 0), genArgs(rules.Conventional()))
	if err == nil {
		t.Fatal("generateOnce() = nil error; two invalid answers must fail")
	}
	if got := exitcode.Of(err); got != exitcode.GenerationFailed {
		t.Errorf("exitcode.Of(err) = %d, want %d", got, exitcode.GenerationFailed)
	}
	if !strings.Contains(err.Error(), "type-enum") {
		t.Errorf("generateOnce() = %v, want the violation named for manual use", err)
	}
}
