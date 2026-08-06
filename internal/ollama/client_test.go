package ollama_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guerrero/gitia/internal/exitcode"
	"github.com/guerrero/gitia/internal/ollama"
)

func TestTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("path = %q, want /api/tags", r.URL.Path)
		}
		fmt.Fprint(w, `{"models":[
			{"name":"gemma4:e2b-it-qat","size":4300000000,"digest":"abc"},
			{"name":"qwen3:4b","size":2600000000,"digest":"def"}
		]}`)
	}))
	defer srv.Close()

	models, err := ollama.New(srv.URL, 5*time.Second).Tags(t.Context())
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("Tags() returned %d models, want 2", len(models))
	}
	if models[0].Name != "gemma4:e2b-it-qat" {
		t.Errorf("models[0].Name = %q, want %q", models[0].Name, "gemma4:e2b-it-qat")
	}
	if models[0].Size != 4300000000 {
		t.Errorf("models[0].Size = %d, want 4300000000", models[0].Size)
	}
}

func TestTagsUnreachableMapsToExitCode5(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // refuse connections

	_, err := ollama.New(srv.URL, time.Second).Tags(t.Context())
	if err == nil {
		t.Fatal("Tags() = nil error against a closed server, want an error")
	}
	if got := exitcode.Of(err); got != exitcode.OllamaUnreachable {
		t.Errorf("exit code = %d, want %d (OllamaUnreachable)", got, exitcode.OllamaUnreachable)
	}
}

func TestHas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"models":[{"name":"gemma4:e2b-it-qat","size":1,"digest":"a"}]}`)
	}))
	defer srv.Close()

	c := ollama.New(srv.URL, 5*time.Second)

	for name, want := range map[string]bool{
		"gemma4:e2b-it-qat": true,
		"gemma4:e2b":        false,
	} {
		got, err := c.Has(t.Context(), name)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("Has(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestHasTreatsABareNameAsTheLatestTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"models":[{"name":"qwen3:latest","size":1,"digest":"a"}]}`)
	}))
	defer srv.Close()

	got, err := ollama.New(srv.URL, 5*time.Second).Has(t.Context(), "qwen3")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error(`Has("qwen3") = false; a bare name must match the :latest tag`)
	}
}

func TestGenerateSendsTheSchemaAndDisablesStreaming(t *testing.T) {
	var got struct {
		Model     string          `json:"model"`
		Prompt    string          `json:"prompt"`
		System    string          `json:"system"`
		Format    json.RawMessage `json:"format"`
		Stream    bool            `json:"stream"`
		KeepAlive string          `json:"keep_alive"`
		Options   struct {
			Temperature float64 `json:"temperature"`
			NumCtx      int     `json:"num_ctx"`
		} `json:"options"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("path = %q, want /api/generate", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(w, `{"model":"m","response":"{\"type\":\"feat\",\"subject\":\"x\"}","done":true,"total_duration":1234}`)
	}))
	defer srv.Close()

	schema := json.RawMessage(`{"type":"object","properties":{"type":{"type":"string"}}}`)

	resp, err := ollama.New(srv.URL, 5*time.Second).Generate(t.Context(), ollama.GenerateRequest{
		Model:     "gemma4:e2b-it-qat",
		Prompt:    "the diff",
		System:    "the rules",
		Format:    schema,
		Stream:    true, // must be forced to false
		KeepAlive: "5m",
		Options:   ollama.Options{Temperature: 0.2, NumCtx: 8192},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if got.Stream {
		t.Error("stream = true; Generate must always request a single non-streamed response")
	}
	if got.Model != "gemma4:e2b-it-qat" {
		t.Errorf("model = %q, want %q", got.Model, "gemma4:e2b-it-qat")
	}
	if got.System != "the rules" {
		t.Errorf("system = %q, want %q", got.System, "the rules")
	}
	if string(got.Format) != string(schema) {
		t.Errorf("format = %s, want %s", got.Format, schema)
	}
	if got.KeepAlive != "5m" {
		t.Errorf("keep_alive = %q, want %q", got.KeepAlive, "5m")
	}
	if got.Options.Temperature != 0.2 || got.Options.NumCtx != 8192 {
		t.Errorf("options = %+v, want temperature 0.2 and num_ctx 8192", got.Options)
	}
	if resp.Response != `{"type":"feat","subject":"x"}` {
		t.Errorf("Response = %q, want the raw JSON body", resp.Response)
	}
}

func TestGenerateServerErrorMapsToExitCode7(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"model requires more system memory"}`)
	}))
	defer srv.Close()

	_, err := ollama.New(srv.URL, 5*time.Second).Generate(t.Context(), ollama.GenerateRequest{Model: "m"})
	if err == nil {
		t.Fatal("Generate() = nil error on a 500, want an error")
	}
	if got := exitcode.Of(err); got != exitcode.GenerationFailed {
		t.Errorf("exit code = %d, want %d (GenerationFailed)", got, exitcode.GenerationFailed)
	}
	if !contains(err.Error(), "more system memory") {
		t.Errorf("error = %q, want it to quote the server's message", err)
	}
}

func TestPullStreamsProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pull" {
			t.Errorf("path = %q, want /api/pull", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"pulling manifest"}`+"\n")
		fmt.Fprint(w, `{"status":"downloading","completed":50,"total":100}`+"\n")
		fmt.Fprint(w, `{"status":"success"}`+"\n")
	}))
	defer srv.Close()

	var seen []ollama.PullProgress
	err := ollama.New(srv.URL, 5*time.Second).Pull(t.Context(), "gemma4:e2b-it-qat",
		func(p ollama.PullProgress) { seen = append(seen, p) })
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if len(seen) != 3 {
		t.Fatalf("Pull reported %d progress events, want 3: %+v", len(seen), seen)
	}
	if seen[1].Completed != 50 || seen[1].Total != 100 {
		t.Errorf("seen[1] = %+v, want completed 50 of 100", seen[1])
	}
	if seen[2].Status != "success" {
		t.Errorf("seen[2].Status = %q, want success", seen[2].Status)
	}
}

func TestPullSurfacesAStreamedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"pulling manifest"}`+"\n")
		fmt.Fprint(w, `{"error":"model not found"}`+"\n")
	}))
	defer srv.Close()

	err := ollama.New(srv.URL, 5*time.Second).Pull(t.Context(), "nope", func(ollama.PullProgress) {})
	if err == nil {
		t.Fatal("Pull() = nil error when the stream carried one, want an error")
	}
	if !contains(err.Error(), "model not found") {
		t.Errorf("error = %q, want it to quote the streamed message", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
