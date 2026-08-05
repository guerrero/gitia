// Package ollama is an HTTP client for a locally running Ollama server. It
// carries no policy: model fallback and pull prompting live in the caller.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/guerrero/gitia/internal/exitcode"
)

// Client talks to one Ollama server.
type Client struct {
	host string
	http *http.Client
}

// New builds a client for host, which must include a scheme.
func New(host string, timeout time.Duration) *Client {
	return &Client{
		host: strings.TrimRight(host, "/"),
		http: &http.Client{Timeout: timeout},
	}
}

// Host returns the base URL, for diagnostics.
func (c *Client) Host() string { return c.host }

// Model is one locally available model.
type Model struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

// Options are the generation parameters gitia sets.
type Options struct {
	Temperature float64 `json:"temperature"`
	NumCtx      int     `json:"num_ctx"`
}

// GenerateRequest is the body of POST /api/generate.
type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	System string `json:"system,omitempty"`
	// Format is a JSON schema. Ollama constrains the decoder to it, not just
	// the prompt, which is what makes a 2B model viable here.
	Format    json.RawMessage `json:"format,omitempty"`
	Stream    bool            `json:"stream"`
	KeepAlive string          `json:"keep_alive,omitempty"`
	Options   Options         `json:"options"`
}

// GenerateResponse is the single object returned when stream is false.
type GenerateResponse struct {
	Model         string `json:"model"`
	Response      string `json:"response"`
	Done          bool   `json:"done"`
	TotalDuration int64  `json:"total_duration"`
}

// PullProgress is one NDJSON line from /api/pull.
type PullProgress struct {
	Status    string `json:"status"`
	Completed int64  `json:"completed"`
	Total     int64  `json:"total"`
	Error     string `json:"error"`
}

type errorBody struct {
	Error string `json:"error"`
}

func (c *Client) post(ctx context.Context, path string, body any) (*http.Response, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+path, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, exitcode.Wrapf(exitcode.OllamaUnreachable,
			"cannot reach Ollama at %s: %v", c.host, err)
	}
	return resp, nil
}

// Tags lists the models available locally.
func (c *Client) Tags(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.host+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, exitcode.Wrapf(exitcode.OllamaUnreachable,
			"cannot reach Ollama at %s: %v", c.host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, exitcode.Wrapf(exitcode.OllamaUnreachable,
			"Ollama returned %s from /api/tags", resp.Status)
	}

	var body struct {
		Models []Model `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, exitcode.Wrapf(exitcode.OllamaUnreachable, "decode /api/tags: %v", err)
	}
	return body.Models, nil
}

// Show returns the local metadata for name, or nil when it is absent.
func (c *Client) Show(ctx context.Context, name string) (*Model, error) {
	models, err := c.Tags(ctx)
	if err != nil {
		return nil, err
	}
	for i := range models {
		if matchesTag(models[i].Name, name) {
			return &models[i], nil
		}
	}
	return nil, nil
}

// Has reports whether name is available locally.
func (c *Client) Has(ctx context.Context, name string) (bool, error) {
	m, err := c.Show(ctx, name)
	return m != nil, err
}

// matchesTag compares model names, treating a bare name as its :latest tag the
// way the ollama CLI does.
func matchesTag(have, want string) bool {
	normalize := func(s string) string {
		if !strings.Contains(s, ":") {
			return s + ":latest"
		}
		return s
	}
	return normalize(have) == normalize(want)
}

// Generate runs one non-streamed completion. Stream is forced to false
// regardless of what the caller set, so the response is a single JSON object.
func (c *Client) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	req.Stream = false

	resp, err := c.post(ctx, "/api/generate", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		var eb errorBody
		_ = json.Unmarshal(data, &eb)
		msg := eb.Error
		if msg == "" {
			msg = strings.TrimSpace(string(data))
		}
		return nil, exitcode.Wrapf(exitcode.GenerationFailed,
			"Ollama returned %s from /api/generate: %s", resp.Status, msg)
	}

	var out GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, exitcode.Wrapf(exitcode.GenerationFailed, "decode /api/generate: %v", err)
	}
	return &out, nil
}

// Pull downloads name, invoking onProgress for every NDJSON line. gitia never
// pulls implicitly; the caller has already asked the user.
func (c *Client) Pull(ctx context.Context, name string, onProgress func(PullProgress)) error {
	// A pull is far slower than the configured request timeout allows, so it
	// runs on a client with no deadline; ctx cancellation still applies.
	pullClient := &http.Client{}
	buf, err := json.Marshal(map[string]any{"model": name, "stream": true})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/api/pull", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := pullClient.Do(req)
	if err != nil {
		return exitcode.Wrapf(exitcode.OllamaUnreachable, "cannot reach Ollama at %s: %v", c.host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return exitcode.Wrapf(exitcode.ModelMissing,
			"pull %s: Ollama returned %s: %s", name, resp.Status, strings.TrimSpace(string(data)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var p PullProgress
		if err := json.Unmarshal(line, &p); err != nil {
			continue // tolerate a malformed progress line
		}
		if p.Error != "" {
			return exitcode.Wrapf(exitcode.ModelMissing, "pull %s: %s", name, p.Error)
		}
		if onProgress != nil {
			onProgress(p)
		}
	}
	if err := scanner.Err(); err != nil {
		return exitcode.Wrapf(exitcode.ModelMissing, "pull %s: %v", name, err)
	}
	return nil
}
