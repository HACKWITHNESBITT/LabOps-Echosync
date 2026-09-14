// EchoSync backend server.
//
// REST + WebSocket surface, Redis Geo proximity index, pgvector similarity
// ranking, edge Llama-3-8B PII firewall, and 15-minute ephemeral matches.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/echosync/backend/internal/api"
	"github.com/echosync/backend/internal/config"
	"github.com/echosync/backend/internal/embed"
	"github.com/echosync/backend/internal/geo"
	"github.com/echosync/backend/internal/match"
	"github.com/echosync/backend/internal/privacy"
	"github.com/echosync/backend/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := run(ctx, cfg); err != nil {
		slog.Error("server terminated", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config) error {
	// --- stores -----------------------------------------------------------
	geoStore, err := geo.New(ctx, cfg.Redis, cfg.Match.TTL)
	if err != nil {
		return err
	}
	defer geoStore.Close()

	vecStore, err := store.New(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer vecStore.Close()

	// --- privacy firewall ---------------------------------------------------
	primary := pickRedactor(cfg)
	pipe := privacy.NewPipeline(primary, nil, cfg.ShowRaw)

	// --- icebreaker + embedding engines -------------------------------------
	gen := match.NewGenerator(cfg.Ollama)
	embedder := embed.New(cfg.Embed.Provider, cfg.Embed.Model, cfg.Ollama.URL)

	// --- server (hub owns engine) -------------------------------------------
	server := api.NewServer(cfg, geoStore, vecStore, pipe, embedder, gen)
	pipe.SetEvents(server.EventSink())

	mux := http.NewServeMux()
	server.Routes(mux)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// --- background engines --------------------------------------------------
	go server.Engine().StartSweeps(ctx)
	go server.Engine().Janitor(ctx)

	slog.Info("echosync backend listening",
		"addr", cfg.HTTPAddr,
		"privacy_pipeline", primary.Name(),
		"pport_ollama", cfg.Ollama.URL,
		"embed_provider", cfg.Embed.Provider,
		"match_radius_m", cfg.Match.RadiusM,
		"match_ttl_s", int(cfg.Match.TTL.Seconds()),
		"min_tokens", cfg.Match.MinTokens,
		"similarity_threshold", cfg.Match.Similarity)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server starting", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}

// pickRedactor selects the PII firewall: rule-based always available; llama
// mode enforces the edge model; auto prefers llama and falls back to rule.
func pickRedactor(cfg config.Config) privacy.Redactor {
	llama := privacy.NewLlamaRedactor(cfg.Ollama, cfg.PII.OllamaScrubPrompt)
	switch cfg.PII.Mode {
	case "rule":
		return privacy.NewRuleRedactor()
	case "llama":
		return llama
	case "auto":
		pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err := llama.Ping(pingCtx)
		if err == nil {
			slog.Info("edge Llama-3-8B inference engine reachable", "model", cfg.Ollama.Model)
			return llama
		}
		slog.Warn("edge Llama-3-8B not reachable — using deterministic rule firewall",
			"url", cfg.Ollama.URL, "err", err)
		return privacy.NewRuleRedactor()
	}
	return privacy.NewRuleRedactor()
}

// logRequests is a tiny access-log middleware.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Debug("http", "method", r.Method, "path", r.URL.Path,
			"status", sw.status, "ms", time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}