package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/echosync/backend/internal/config"
	"github.com/echosync/backend/internal/embed"
	"github.com/echosync/backend/internal/geo"
	"github.com/echosync/backend/internal/match"
	"github.com/echosync/backend/internal/model"
	"github.com/echosync/backend/internal/privacy"
	"github.com/echosync/backend/internal/store"
)

// Server composes all subsystems behind one HTTP/WS surface.
type Server struct {
	cfg     config.Config
	geo     *geo.Store
	vec     *store.VectorStore
	privacy *privacy.Pipeline
	emb     embed.Embedder
	engine  *match.Engine
	hub     *Hub

	eventsMu sync.Mutex
	events   []model.ScrubEvent // ring for command-center polling
	eventsMax int
}

func NewServer(cfg config.Config,
	g *geo.Store, ps *store.VectorStore,
	pipe *privacy.Pipeline, emb embed.Embedder, gen *match.Generator,
) *Server {
	s := &Server{
		cfg:       cfg,
		geo:       g,
		vec:       ps,
		privacy:   pipe,
		emb:       emb,
		hub:       NewHub(30 * time.Second),
		eventsMax: 100,
	}
	s.hub.SetHandler(s.handleWSMsg)
	s.engine = match.NewEngine(g, ps, emb, gen, cfg.Match, s.hub)
	return s
}

func (s *Server) Hub() *Hub { return s.hub }

// Engine exposes the matching engine for background sweeps/janitors.
func (s *Server) Engine() *match.Engine { return s.engine }

func (s *Server) EventSink() privacy.EventSink {
	return func(ev privacy.Event) {
		s.hub.Pipeline(ev)
		s.recordEvent(ev)
	}
}

func (s *Server) recordEvent(ev privacy.Event) {
	scrub := model.ScrubEvent{
		ID: ev.ID, Type: ev.Type, UserID: ev.UserID,
		Pipeline: ev.Processor, RawLength: ev.RawLen, RedactedLen: ev.RedactLen,
		Tokens: ev.Tokens, Blocked: ev.Blocked, At: ev.At,
	}
	if ev.Raw != "" {
		scrub.Raw = ev.Raw
	}
	s.eventsMu.Lock()
	s.events = append(s.events, scrub)
	if len(s.events) > s.eventsMax {
		s.events = s.events[len(s.events)-s.eventsMax:]
	}
	s.eventsMu.Unlock()
}

// Routes registers every endpoint on the given mux.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/metrics", s.metrics)
	mux.HandleFunc("/ws", s.ws)
	mux.HandleFunc("/v1/scrub", s.scrub)
	mux.HandleFunc("/v1/presence", s.presence)
	mux.HandleFunc("/v1/presence/", s.presence)
	mux.HandleFunc("/v1/matches/", s.matches)
	mux.HandleFunc("/v1/icebreaker", s.icebreaker)
	mux.HandleFunc("/v1/radar/", s.radar)
	mux.HandleFunc("/v1/pipeline/events", s.pipelineEvents)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"service":     "echosync-backend",
		"radar_radius_m": s.cfg.Match.RadiusM,
		"match_ttl_s": int(s.cfg.Match.TTL.Seconds()),
		"time":        time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	m := s.engine.Metrics(r.Context())
	if s.privacy != nil {
		runs, blocked := s.privacy.Stats()
		m.ScrubsRun, m.PIIBlocked = runs, blocked
		m.Pipeline = s.privacy.ActivePipeline()
	}
	writeJSON(w, http.StatusOK, m)
}

// --- REST handlers --------------------------------------------------------

func (s *Server) scrub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.ScrubRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(s.cfg.PII.MaxBytes+1024))).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid scrub request: "+err.Error())
		return
	}
	req.Transcript = strings.TrimSpace(req.Transcript)
	if len(req.Transcript) > s.cfg.PII.MaxBytes {
		writeErr(w, http.StatusUnprocessableEntity, "transcript exceeds max size")
		return
	}
	if req.UserID == "" {
		req.UserID = "anon"
	}

	scrubStart := time.Now()
	res, err := s.privacy.ScrubAndTokenize(r.Context(), req.UserID, req.Transcript)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "privacy pipeline failed: "+err.Error())
		return
	}

	// Generate + persist the vector only AFTER PII has been stripped — the
	// embedding space never sees raw identifiers.
	if s.emb != nil {
		if v, err := s.emb.Embed(r.Context(), res.Tokens); err == nil {
			_ = s.vec.UpsertVector(r.Context(), req.UserID, res.Tokens, v)
		}
	}
	embedLatency := time.Since(scrubStart) - res.ScrubLatency

	writeJSON(w, http.StatusCreated, model.ScrubResponse{
		Pipeline:      res.Pipeline,
		Tokens:        res.Tokens,
		BlockedPII:    res.BlockedPII,
		RedactedLen:   res.RedactedLen,
		ScrubLatencyMs: res.ScrubLatency.Milliseconds(),
		EmbedLatencyMs: embedLatency.Milliseconds(),
	})
}

func (s *Server) presence(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.presenceCreate(w, r)
	case http.MethodDelete:
		s.presenceDelete(w, r)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "POST or DELETE required")
	}
}

func (s *Server) presenceCreate(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var p model.Presence
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid presence: "+err.Error())
		return
	}
	p.UserID = strings.TrimSpace(p.UserID)
	p.Interests = dedupeUpperTokens(p.Interests)
	if p.UserID == "" {
		writeErr(w, http.StatusBadRequest, "user_id required")
		return
	}
	created, err := s.engine.ProcessPresence(r.Context(), p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "presence processing failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":   p.UserID,
		"processed_ms": time.Since(start).Milliseconds(),
		"matches":   created,
	})
}

func (s *Server) presenceDelete(w http.ResponseWriter, r *http.Request) {
	userID := lastSegment(r.URL.Path)
	if userID == "" {
		writeErr(w, http.StatusBadRequest, "user_id required in path")
		return
	}
	// A user leaving clears its geo position. (Redis key removal; vector row
	// stays for the session stats — a real deployment would soft-delete.)
	if err := s.geo.RemoveUser(r.Context(), userID); err != nil && !errors.Is(err, geo.ErrNotRegistered) {
		slog.Warn("presence delete", "err", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "status": "gone"})
}

func (s *Server) matches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	userID := lastSegment(r.URL.Path)
	if userID == "" {
		writeErr(w, http.StatusBadRequest, "user_id required in path")
		return
	}
	recs, err := s.geo.MatchesFor(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "matches query failed")
		return
	}
	out := make([]model.Match, 0, len(recs))
	for _, rec := range recs {
		out = append(out, toMatch(rec, userID, s.cfg.Match.TTL.Seconds()))
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "matches": out, "ttl_seconds": int(s.cfg.Match.TTL.Seconds())})
}

func (s *Server) icebreaker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.IcebreakerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid icebreaker request: "+err.Error())
		return
	}
	rec, err := s.geo.GetMatch(r.Context(), req.MatchID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "ephemeral match not found or expired")
		return
	}
	other := peerOf(rec, req.UserID)
	prompt, source, latency := s.engine.Generator().Generate(r.Context(), rec.SharedTokens, rec.DistanceM, other)
	writeJSON(w, http.StatusOK, model.IcebreakerResponse{
		Icebreaker: prompt,
		Source:     source,
		LatencyMs:  latency,
	})
}

func (s *Server) radar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	userID := lastSegment(r.URL.Path)
	if userID == "" {
		writeErr(w, http.StatusBadRequest, "user_id required in path")
		return
	}
	state, err := s.engine.BuildRadar(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no radar state for user")
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) pipelineEvents(w http.ResponseWriter, r *http.Request) {
	s.eventsMu.Lock()
	events := make([]model.ScrubEvent, len(s.events))
	copy(events, s.events)
	s.eventsMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// --- WebSocket -----------------------------------------------------------

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	s.hub.serveWS(w, r)
}

// handleWSMsg dispatches client->server frames (presence / radar scan).
func (s *Server) handleWSMsg(msg model.WSClientMsg, c *client) {
	switch msg.Type {
	case "ping":
		c.conn.WriteJSON(model.WSServerMsg{Type: "pong"})
	case "presence":
		go func() {
			start := time.Now()
			_, err := s.engine.ProcessPresence(context.Background(), model.Presence{
				UserID:    c.userID,
				Lat:       msg.Lat,
				Lng:       msg.Lng,
				Interests: msg.Interests,
			})
			if err != nil {
				slog.Warn("ws presence failed", "user", c.userID, "err", err)
				return
			}
			slog.Debug("ws presence processed", "user", c.userID, "ms", time.Since(start).Milliseconds())
		}()
	case "scan":
		if state, err := s.engine.BuildRadar(context.Background(), c.userID); err == nil {
			s.hub.push(c, model.WSServerMsg{Type: "radar", Radar: state})
		}
	}
}

// peerOf returns the other participant of a match record.
func peerOf(rec *geo.MatchRecord, self string) string {
	if rec.UserA == self {
		return rec.UserB
	}
	return rec.UserA
}

// --- shared helpers -------------------------------------------------------

func toMatch(rec geo.MatchRecord, forUser string, ttlSeconds float64) model.Match {
	ttl := time.Until(rec.ExpiresAt)
	if ttl < 0 {
		ttl = 0
	}
	return model.Match{
		ID:           rec.ID,
		PeerUserID:   peerOf(&rec, forUser),
		SharedTokens: rec.SharedTokens,
		Similarity:   rec.Similarity,
		DistanceM:    rec.DistanceM,
		Icebreaker:   rec.Icebreaker,
		CreatedAt:    rec.CreatedAt,
		ExpiresAt:    rec.ExpiresAt,
		TTLSeconds:   int64(ttl.Seconds()),
	}
}

func lastSegment(path string) string {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	return parts[len(parts)-1]
}

func dedupeUpperTokens(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, t := range in {
		t = strings.ToUpper(strings.Join(strings.Fields(t), " "))
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode failed", "err", err)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}