package match

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/echosync/backend/internal/config"
)

// Creator generates icebreaker prompts. It prefers the edge Llama-3-8B engine
// and transparently falls back to a template so the system stays operational
// when the model container is cold.
type Creator interface {
	Generate(ctx context.Context, sharedTokens []string, distanceM float64, peerID string) (prompt, source string, latencyMs int64)
	status() (model string, active bool)
}

// Generator implements Creator against Ollama's /api/generate endpoint.
type Generator struct {
	client *http.Client
	url    string
	model  string
	active atomic.Bool
}

func NewGenerator(cfg config.OllamaConfig) *Generator {
	return &Generator{
		client: &http.Client{Timeout: cfg.Timeout},
		url:    cfg.URL,
		model:  cfg.Model,
	}
}

func (g *Generator) status() (string, bool) {
	return g.model, g.active.Load()
}

// Generate returns an icebreaker for a newly created proximity match.
func (g *Generator) Generate(ctx context.Context, sharedTokens []string, distanceM float64, peerID string) (string, string, int64) {
	start := time.Now()
	prompt := g.llm(ctx, sharedTokens, distanceM, peerID)
	if prompt != "" {
		g.active.Store(true)
		return prompt, "llama", time.Since(start).Milliseconds()
	}
	g.active.Store(false)
	return templatePrompt(sharedTokens, distanceM), "template", time.Since(start).Milliseconds()
}

func (g *Generator) llm(ctx context.Context, shared []string, dist float64, peerID string) string {
	if len(shared) == 0 || g.client == nil || g.url == "" {
		return ""
	}
	sys := "You are EchoSync's icebreaker engine. Given only non-identifying shared interest tokens, " +
		"write one short, natural, conversation-starting question in under 30 words. Never invent names or personal data."
	user := fmt.Sprintf("Shared interests: %s. Distance: about %.0f metres away. Peer is anonymous. Question:",
		strings.Join(shared, ", "), dist)
	body, _ := json.Marshal(map[string]any{
		"model": g.model, "prompt": user, "system": sys,
		"stream": false, "options": map[string]any{"temperature": 0.7, "num_predict": 80},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		slog.Debug("icebreaker ollama unreachable, using template", "err", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var gen struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gen); err != nil {
		return ""
	}
	out := strings.Join(strings.Fields(strings.TrimSpace(gen.Response)), " ")
	if out == "" || out == "[]" {
		return ""
	}
	return out
}

// templatePrompt is the deterministic offline fallback.
func templatePrompt(shared []string, dist float64) string {
	top := shared[0]
	subject := strings.ToLower(top)
	switch {
	case len(shared) >= 2:
		return fmt.Sprintf(`You both light up around "%s" and "%s". Ask which got them hooked first — then swap your own origin story.`, shared[0], shared[1])
	case dist < 8:
		return fmt.Sprintf(`You're only ~%.0f m apart and both into %s. Ask the biggest misconception most people have about it.`, dist, subject)
	default:
		return fmt.Sprintf(`So you're both into %s — ask what a perfect afternoon spent on it looks like for them.`, subject)
	}
}