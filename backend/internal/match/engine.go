// Package match is EchoSync's proximity + ephemeral matching engine.
//
// # 15-Minute Ephemeral TTL
//
// Every created match is written to Redis with EXPIRE = TTL (default 900s).
// Redis enforces the self-destruct automatically, so match feeds shrink the
// moment a card's TTL elapses — no janitor required. An in-memory expiry
// mirror powers /metrics counters and TTL broadcasts.
package match

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/echosync/backend/internal/config"
	"github.com/echosync/backend/internal/embed"
	"github.com/echosync/backend/internal/geo"
	"github.com/echosync/backend/internal/model"
	"github.com/echosync/backend/internal/store"
)

// Notifier receives events once the engine writes a match/radar update. It is
// wired to the WebSocket hub by the API layer, keeping the engine transport-free.
type Notifier interface {
	NotifyMatch(userID string, m *model.Match, latencyMs int64)
	NotifyRadar(userID string, state *model.RadarState)
}

const radDeg = math.Pi / 180

type Engine struct {
	geo *geo.Store
	vec *store.VectorStore
	emb embed.Embedder
	gen *Generator
	cfg config.MatchConfig

	notifier Notifier

	mtx          sync.Mutex
	expiry       map[string]time.Time
	matchesMade  atomic.Int64
	matchesGone  atomic.Int64
	radarSweepHz float64
	startedAt    time.Time
}

func NewEngine(g *geo.Store, ps *store.VectorStore, emb embed.Embedder, gen *Generator, cfg config.MatchConfig, not Notifier) *Engine {
	return &Engine{
		geo:          g,
		vec:          ps,
		emb:          emb,
		gen:          gen,
		cfg:          cfg,
		notifier:     not,
		expiry:       map[string]time.Time{},
		radarSweepHz: 0.25, // 4s sweep, matches the Flutter radar
		startedAt:    time.Now(),
	}
}

// Generator exposes the icebreaker engine (route handler + tests).
func (e *Engine) Generator() *Generator { return e.gen }

// ProcessPresence upserts a user's proximity + interests, searches the
// 15-meter geofence for peers, ranks them by pgvector cosine similarity and
// creates ephemeral matches for strong overlaps. Latency from proximity
// detection to broadcast is measured end-to-end.
func (e *Engine) ProcessPresence(ctx context.Context, p model.Presence) ([]model.Match, error) {
	start := time.Now()

	if err := e.geo.UpsertPosition(ctx, p.UserID, p.Lat, p.Lng); err != nil {
		return nil, err
	}
	vec, err := e.emb.Embed(ctx, p.Interests)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if err := e.vec.UpsertVector(ctx, p.UserID, p.Interests, vec); err != nil {
		return nil, fmt.Errorf("pgvector upsert: %w", err)
	}

	neighbors, err := e.geo.Nearby(ctx, p.UserID, e.cfg.RadiusM)
	if err != nil {
		return nil, err
	}
	var cands []store.Candidate
	if len(neighbors) > 0 {
		cands, err = e.vec.CandidatesWithSimilarity(ctx, neighbors, vec)
		if err != nil {
			return nil, err
		}
	}

	// Radar sweep broadcast for the reporting client.
	e.notifyRadar(ctx, p.UserID, cands)

	// Match candidate pairs within the geofence.
	var created []model.Match
	for _, c := range cands {
		if c.UserID == p.UserID {
			continue
		}
		shared := intersect(p.Interests, c.Interests)
		if len(shared) < e.cfg.MinTokens {
			continue
		}
		if c.Similarity < e.cfg.Similarity {
			continue
		}
		id := matchID(p.UserID, c.UserID)
		if _, err := e.geo.GetMatch(ctx, id); err == nil {
			continue // already matched; don't re-trigger haptics
		}
		dist, derr := e.geo.DistanceBetween(ctx, p.UserID, c.UserID)
		if derr != nil {
			dist = 0
		}
		ice, src, genLatency := e.gen.Generate(ctx, shared, dist, c.UserID)

		rec := geo.MatchRecord{
			ID: id, UserA: p.UserID, UserB: c.UserID,
			SharedTokens: shared, Similarity: c.Similarity, DistanceM: dist,
			Icebreaker: ice, CreatedAt: start, ExpiresAt: start.Add(e.cfg.TTL),
		}
		if err := e.geo.CreateMatch(ctx, rec); err != nil {
			slog.Error("create match failed", "err", err)
			continue
		}
		e.mtx.Lock()
		e.expiry[id] = rec.ExpiresAt
		e.mtx.Unlock()
		e.matchesMade.Add(1)

		latencyMs := time.Since(start).Milliseconds()
		slog.Info("ephemeral match created",
			"id", id, "a", p.UserID, "b", c.UserID,
			"distance_m", round1(dist), "similarity", round3(c.Similarity),
			"ttl_s", int(e.cfg.TTL.Seconds()),
			"proximity->haptic_latency_ms", latencyMs,
			"icebreaker_ms", genLatency, "icebreaker_source", src)

		m := model.Match{
			ID: id, PeerUserID: c.UserID, SharedTokens: shared,
			Similarity: c.Similarity, DistanceM: dist, Icebreaker: ice,
			CreatedAt: rec.CreatedAt, ExpiresAt: rec.ExpiresAt,
			TTLSeconds: int64(e.cfg.TTL.Seconds()),
		}
		if e.notifier != nil {
			e.notifier.NotifyMatch(p.UserID, &m, latencyMs)
			p2 := m
			p2.PeerUserID = p.UserID
			e.notifier.NotifyMatch(c.UserID, &p2, latencyMs)
		}
		created = append(created, m)
	}
	return created, nil
}

// BuildRadar returns the current radar state for a connected dashboard/app.
func (e *Engine) BuildRadar(ctx context.Context, userID string) (*model.RadarState, error) {
	vec, err := e.vec.EmbeddingFor(ctx, userID)
	if err != nil || len(vec) == 0 {
		interests, ierr := e.vec.InterestsFor(ctx, userID)
		if ierr != nil {
			return nil, ierr
		}
		vec, err = e.emb.Embed(ctx, interests)
		if err != nil {
			return nil, err
		}
	}
	neighbors, err := e.geo.Nearby(ctx, userID, e.cfg.RadiusM)
	if err != nil {
		return nil, err
	}
	cands, err := e.vec.CandidatesWithSimilarity(ctx, neighbors, vec)
	if err != nil {
		return nil, err
	}
	state, err := e.buildTargets(ctx, userID, cands)
	if err != nil {
		return nil, err
	}
	return state, nil
}

func (e *Engine) notifyRadar(ctx context.Context, userID string, cands []store.Candidate) {
	if e.notifier == nil {
		return
	}
	state, err := e.buildTargets(ctx, userID, cands)
	if err == nil {
		e.notifier.NotifyRadar(userID, state)
	}
}

// buildTargets converts geo neighbors into radar targets with 3D vector links.
func (e *Engine) buildTargets(ctx context.Context, self string, cands []store.Candidate) (*model.RadarState, error) {
	var targets []model.RadarTarget
	for _, c := range cands {
		if c.UserID == self {
			continue
		}
		bearing, err := e.geo.BearingBetween(ctx, self, c.UserID)
		if err != nil {
			continue
		}
		dist, err := e.geo.DistanceBetween(ctx, self, c.UserID)
		if err != nil {
			continue
		}
		scaled := 4.0 + (dist/e.cfg.RadiusM)*118.0
		rad := bearing * radDeg

		targets = append(targets, model.RadarTarget{
			ID:         c.UserID,
			DistanceM:  round1(dist),
			BearingDeg: round1(bearing),
			Shared:     c.Interests,
			Similarity: c.Similarity,
			Link: [2]model.Vec3{
				{0, 0, 0},
				{normF32(scaled * math_cos(rad)), normF32(scaled * math_sin(rad)), normF32(dist)},
			},
			FirstSeen: time.Now(),
		})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].DistanceM < targets[j].DistanceM })
	return &model.RadarState{
		Self:       model.Vec3{0, 0, 0},
		Targets:    targets,
		RadiusM:    e.cfg.RadiusM,
		SweepHz:    e.radarSweepHz,
		PipelineID: self,
	}, nil
}

// Janitor mirrors the Redis-enforced TTL for dashboard counters.
func (e *Engine) Janitor(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			e.mtx.Lock()
			for id, exp := range e.expiry {
				if now.After(exp) {
					delete(e.expiry, id)
					e.matchesGone.Add(1)
					slog.Debug("ephemeral match expired via TTL", "id", id, "ttl_s", int(e.cfg.TTL.Seconds()))
				}
			}
			e.mtx.Unlock()
		}
	}
}

// StartSweeps pushes live countdown updates for active matches so clients can
// render the ephemeral TTL without polling.
func (e *Engine) StartSweeps(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			e.mtx.Lock()
			ids := make([]string, 0, len(e.expiry))
			for id := range e.expiry {
				ids = append(ids, id)
			}
			e.mtx.Unlock()
			if e.notifier == nil {
				continue
			}
			for _, id := range ids {
				rec, err := e.geo.GetMatch(ctx, id)
				if err != nil {
					continue
				}
				ttl := time.Until(rec.ExpiresAt)
				if ttl < 0 {
					ttl = 0
				}
				upd := &model.Match{
					ID: rec.ID, SharedTokens: rec.SharedTokens, Similarity: rec.Similarity,
					DistanceM: rec.DistanceM, Icebreaker: rec.Icebreaker,
					CreatedAt: rec.CreatedAt, ExpiresAt: rec.ExpiresAt,
					TTLSeconds: int64(ttl.Seconds()),
				}
				updA, updB := *upd, *upd
				updA.PeerUserID = rec.UserB
				updB.PeerUserID = rec.UserA
				e.notifier.NotifyMatch(rec.UserA, &updA, 0)
				e.notifier.NotifyMatch(rec.UserB, &updB, 0)
			}
		}
	}
}

// Metrics aggregates store + engine counters for /metrics and the dashboard.
func (e *Engine) Metrics(ctx context.Context) model.Metrics {
	avg, p95 := e.geo.LatencyStats()
	users, _ := e.geo.ActiveUserCount(ctx)
	activeMatches, _ := e.geo.ActiveMatchCount(ctx)
	modelName := ""
	llmActive := false
	if e.gen != nil {
		modelName, llmActive = e.gen.status()
	}
	return model.Metrics{
		ActiveUsers:       users,
		ActiveMatches:     activeMatches,
		MatchesCreated:    e.matchesMade.Load(),
		MatchesExpired:    e.matchesGone.Load(),
		Pipeline:          "rule",
		AvgMatchLatencyMs:  avg,
		P95MatchLatencyMs: p95,
		MatchTTLSeconds:   int64(e.cfg.TTL.Seconds()),
		MatchRadiusM:      e.cfg.RadiusM,
		UptimeSeconds:     time.Since(e.startedAt).Seconds(),
		OllamaModel:       modelName,
		OllamaActive:      llmActive,
	}
}

// helpers ------------------------------------------------------------------

func matchID(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("%s:%s", a, b)
}

func intersect(a, b []string) []string {
	bset := make(map[string]struct{}, len(b))
	for _, x := range b {
		bset[x] = struct{}{}
	}
	var out []string
	for _, x := range a {
		if _, ok := bset[x]; ok {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }
func round3(f float64) float64 { return math.Round(f*1000) / 1000 }
func normF32(f float64) float64 { return f }

// Taylor-series trig (deterministic, dependency-free) — fine for radar layout.
func math_cos(r float64) float64 {
	r2 := r * r
	return 1 - r2/2 + r2*r2/24 - r2*r2*r2/720
}
func math_sin(r float64) float64 {
	r2 := r * r
	return r - r2*r/6 + r2*r2*r/120 - r2*r2*r2*r/5040
}