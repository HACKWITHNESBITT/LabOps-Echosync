package privacy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/echosync/backend/internal/config"
)

// LlamaRedactor drives an edge/on-device Llama-3-8B instance through the
// Ollama HTTP API. Prompting instructs the model to act as a zero-trust PII
// firewall that returns ONLY a JSON array of interest tokens.
type LlamaRedactor struct {
	client *http.Client
	url    string
	model  string
	system string
	prompt string
}

func NewLlamaRedactor(cfg config.OllamaConfig, scrubPrompt string) *LlamaRedactor {
	return &LlamaRedactor{
		client: &http.Client{Timeout: cfg.Timeout},
		url:    cfg.URL,
		model:  cfg.Model,
		system: cfg.BaseSystem,
		prompt: scrubPrompt,
	}
}

func (r *LlamaRedactor) Name() string { return PipelineLlama }

// Ping verifies the edge inference engine is reachable and knows the model.
func (r *LlamaRedactor) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama status %d", resp.StatusCode)
	}
	return nil
}

// Scrub asks Llama-3 to scrub PII and extract interest tokens.
func (r *LlamaRedactor) Scrub(ctx context.Context, transcript string) (ScrubResult, error) {
	start := time.Now()
	user := fmt.Sprintf("Transcript: %q\nExtract the non-identifying interest tokens as a JSON array.", truncate(transcript, 2000))
	reqBody := ollamaGenerateRequest{
		Model:     r.model,
		Prompt:    user,
		System:    r.system + "\n\n" + r.prompt,
		Stream:    false,
		Format:    "json",
		Options:   map[string]any{"temperature": 0.1},
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return ScrubResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return ScrubResult{}, fmt.Errorf("llama generate: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ScrubResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		// Surface Ollama/model-not-found gracefully.
		return ScrubResult{}, &httpError{status: resp.StatusCode, body: strings.TrimSpace(string(raw))}
	}

	var gen ollamaGenerateResponse
	if err := json.Unmarshal(raw, &gen); err != nil {
		return ScrubResult{}, fmt.Errorf("llama decode: %w", err)
	}

	var tokens []string
	if err := json.Unmarshal([]byte(gen.Response), &tokens); err != nil {
		// model may return a JSON object with "tokens"; be lenient
		var obj map[string]any
		if e2 := json.Unmarshal([]byte(gen.Response), &obj); e2 == nil {
			if arr, ok := obj["tokens"].([]any); ok {
				for _, v := range arr {
					if s, ok := v.(string); ok {
						tokens = append(tokens, normalize(s))
					}
				}
			}
		}
	}
	tokens = dedupeUpper(tokens)
	return ScrubResult{
		Tokens:       tokens,
		Pipeline:     PipelineLlama,
		ScrubLatency: time.Since(start),
	}, nil
}

type ollamaGenerateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	System  string         `json:"system"`
	Stream  bool           `json:"stream"`
	Format  string         `json:"format"`
	Options map[string]any `json:"options"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

type httpError struct {
	status int
	body   string
}

func (e *httpError) Error() string {
	if n := len(e.body); n > 160 {
		return fmt.Sprintf("ollama http %d: %s…", e.status, e.body[:160])
	}
	return fmt.Sprintf("ollama http %d: %s", e.status, e.body)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func dedupeUpper(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, t := range in {
		t = normalize(strings.Trim(t, `"'`))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}