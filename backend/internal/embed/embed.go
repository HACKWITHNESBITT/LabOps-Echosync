// Package embed produces deterministic interest vectors. It runs after the
// PII firewall, so no private identifiers ever reach the embedding space.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
)

const Dim = 384

// Embedder turns curated interest tokens into a normalized vector.
type Embedder interface {
	Embed(ctx context.Context, tokens []string) ([]float32, error)
}

// New builds the configured embedder. "ollama" uses the edge engine's
// embedding endpoint; anything else uses the deterministic hash embedder.
func New(provider, model, ollamaURL string) Embedder {
	if provider == "ollama" && ollamaURL != "" {
		return &OllamaEmbedder{client: &http.Client{}, url: ollamaURL, model: model}
	}
	return &HashEmbedder{}
}

// HashEmbedder maps tokens into a 384-dim space with a per-token feature hash
// and IDF-like normalization. It is fully offline and deterministic, which
// makes tests reproducible — while Ollama/nomic embeddings transparently
// upgrade the same interface in production.
type HashEmbedder struct{}

func (h *HashEmbedder) Embed(_ context.Context, tokens []string) ([]float32, error) {
	vec := make([]float32, Dim)
	for _, tok := range tokens {
		t := strings.ToLower(tok)
		if t == "" {
			continue
		}
		hash := uint64(0xcbf29ce484222325)
		for i := 0; i < len(t); i++ {
			hash ^= uint64(t[i])
			hash *= 0x100000001b3
		}
		// two independent hashes -> one feature index via double hashing
		idx1 := int(hash % Dim)
		vec[idx1] += 1
		for k := uint64(1); k < 3; k++ {
			hash ^= hash >> 33
			hash *= 0xff51afd7ed558ccd
			hash ^= hash >> 33
			idx2 := int((hash + k*0x9e3779b97f4a7c15) % Dim)
			vec[idx2] += 0.5
		}
	}
	// normalize to unit length (cosine metric)
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm == 0 {
		return vec, nil
	}
	inv := float32(1 / math.Sqrt(norm))
	for i := range vec {
		vec[i] *= inv
	}
	return vec, nil
}

// OllamaEmbedder queries the edge engine's /api/embeddings endpoint.
type OllamaEmbedder struct {
	client *http.Client
	url    string
	model  string
}

func (o *OllamaEmbedder) Embed(ctx context.Context, tokens []string) ([]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"model":   o.model,
		"prompt":  strings.Join(tokens, " "),
		"options": map[string]any{"temperature": 0},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.url+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embeddings status %d", resp.StatusCode)
	}
	var out struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	vec := make([]float32, len(out.Embedding))
	for i, v := range out.Embedding {
		vec[i] = float32(v)
	}
	return vec, nil
}