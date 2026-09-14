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

// GroqRedactor runs the zero-trust PII firewall against an OpenAI-compatible
// chat-completions endpoint (Groq / Together AI). The model is an open-weight
// Llama-3-8B-class model configured via GROQ_MODEL. Prompting instructs the
// model to strip identifiers and return ONLY a JSON array of interest tokens —
// raw PII never survives into the vector stage.
type GroqRedactor struct {
	client  *http.Client
	apiKey  string
	baseURL string
	model   string
	system  string
	prompt  string
}

func NewGroqRedactor(cfg config.GroqConfig, scrubPrompt string) *GroqRedactor {
	return &GroqRedactor{
		client:  &http.Client{Timeout: cfg.Timeout},
		apiKey:  cfg.APIKey,
		baseURL: cfg.BaseURL,
		model:   cfg.Model,
		system:  "You are EchoSync's privacy firewall. You extract non-identifying interest keywords from ambient speech and absolutely never repeat names, emails, phone numbers, addresses, employers or other PII.",
		prompt:  scrubPrompt,
	}
}

func (r *GroqRedactor) Name() string { return PipelineGroq }

// Ping verifies the API key and model are usable.
func (r *GroqRedactor) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("groq unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("groq status %d", resp.StatusCode)
	}
	return nil
}

// Scrub asks the model to scrub PII and extract interest tokens as JSON.
func (r *GroqRedactor) Scrub(ctx context.Context, transcript string) (ScrubResult, error) {
	start := time.Now()
	reqBody := groqChatRequest{
		Model: r.model,
		Messages: []groqMessage{
			{Role: "system", Content: r.system + "\n\n" + r.prompt},
			{Role: "user", Content: "Transcript: " + truncate(transcript, 2000)},
		},
		Temperature: 0.0,
		MaxTokens:   400,
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ScrubResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return ScrubResult{}, fmt.Errorf("groq chat: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ScrubResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ScrubResult{}, &groqHTTPError{status: resp.StatusCode, body: strings.TrimSpace(string(raw))}
	}

	var chat groqChatResponse
	if err := json.Unmarshal(raw, &chat); err != nil {
		return ScrubResult{}, fmt.Errorf("groq decode: %w", err)
	}
	if len(chat.Choices) == 0 {
		return ScrubResult{}, fmt.Errorf("groq returned no choices")
	}
	content := chat.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		return ScrubResult{}, fmt.Errorf("groq returned empty content")
	}

	tokens := parseTokenJSON(content)
	tokens = dedupeUpper(tokens)
	return ScrubResult{
		Tokens:       tokens,
		BlockedPII:   nil,
		RedactedLen:  0,
		Pipeline:     PipelineGroq,
		ScrubLatency: time.Since(start),
	}, nil
}

type groqChatRequest struct {
	Model       string        `json:"model"`
	Messages    []groqMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// parseTokenJSON leniently parses a bare array or a {"tokens":[...]} object,
// tolerating markdown fences and surrounding prose.
func parseTokenJSON(s string) []string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			s = s[i : j+1]
		}
	}
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		return arr
	}
	var obj struct {
		Tokens []string `json:"tokens"`
	}
	_ = json.Unmarshal([]byte(s), &obj)
	return obj.Tokens
}

type groqHTTPError struct {
	status int
	body   string
}

func (e *groqHTTPError) Error() string {
	if n := len(e.body); n > 160 {
		return fmt.Sprintf("groq http %d: %s…", e.status, e.body[:160])
	}
	return fmt.Sprintf("groq http %d: %s", e.status, e.body)
}