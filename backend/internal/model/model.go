// Package model defines the core EchoSync wire types shared across the
// REST API, WebSocket protocol and storage layers.
package model

import "time"

// PII pipeline events -------------------------------------------------------

const (
	EventScrubStarted  = "scrub.start"
	EventScrubScrubbed = "scrub.scrubbed"
	EventScrubEmbedded = "scrub.embedded"
	EventScrubStored   = "scrub.stored"
	EventScrubFallback = "scrub.fallback"
	EventMatchCreated  = "match.created"
	EventMatchExpired  = "match.expired"
)

// ScrubEvent describes one privacy-pipeline hop. It is emitted over the
// command-center WebSocket for the live PII visualizer. The raw transcript
// is ONLY transmitted when ShowRaw is enabled (dev/demo), never stored.
type ScrubEvent struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	UserID      string    `json:"user_id"`
	Pipeline    string    `json:"pipeline"` // llama | rule
	Source      string    `json:"source,omitempty"`
	RawLength   int       `json:"raw_length"`
	RedactedLen int       `json:"redacted_len,omitempty"`
	Tokens      []string  `json:"tokens,omitempty"`
	Blocked     []string  `json:"blocked,omitempty"`
	Raw         string    `json:"raw,omitempty"` // dev/demo only
	At          time.Time `json:"at"`
}

// Presence ---------------------------------------------------------------

type Presence struct {
	UserID    string   `json:"user_id"`
	Lat       float64  `json:"lat"`
	Lng       float64  `json:"lng"`
	Interests []string `json:"interests"`
}

// ScrubRequest / ScrubResponse --------------------------------------------

type ScrubRequest struct {
	UserID     string `json:"user_id"`
	Transcript string `json:"transcript"`
	// Audio is intentionally NOT part of transport: raw buffers exist only in
	// RAM on the edge device. text is the result of on-device ASR.
}

type ScrubResponse struct {
	Pipeline     string   `json:"pipeline"`
	Tokens       []string `json:"tokens"`
	BlockedPII   []string `json:"blocked_pii"`
	RedactedLen  int      `json:"redacted_len"`
	ScrubLatencyMs int64    `json:"scrub_latency_ms"`
	EmbedLatencyMs int64    `json:"embed_latency_ms"`
}

// Match ---------------------------------------------------------------

type Match struct {
	ID           string   `json:"id"`
	PeerUserID   string   `json:"peer_user_id"`
	SharedTokens []string `json:"shared_tokens"`
	Similarity   float64  `json:"similarity"`
	DistanceM    float64  `json:"distance_m"`
	Icebreaker   string   `json:"icebreaker"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	TTLSeconds   int64    `json:"ttl_seconds"`
}

// RadarTarget describes a nearby peer for the sonar radar, plus the vector
// link line from self.
type RadarTarget struct {
	ID         string   `json:"id"`
	DistanceM  float64  `json:"distance_m"`
	BearingDeg float64  `json:"bearing_deg"` // 0=N 90=E
	Shared     []string `json:"shared_tokens,omitempty"`
	Similarity float64  `json:"similarity,omitempty"`
	Link       [2]Vec3  `json:"link"` // xyz endpoints of the vector link
	FirstSeen  time.Time `json:"first_seen"`
}

type Vec3 [3]float64

type RadarState struct {
	Self       Vec3          `json:"self"`
	Targets    []RadarTarget `json:"targets"`
	RadiusM    float64       `json:"radius_m"`
	SweepHz    float64       `json:"sweep_hz"`
	PipelineID string        `json:"pipeline_id"`
}

// Icebreaker --------------------------------------------------------------

type IcebreakerRequest struct {
	UserID  string `json:"user_id"`
	MatchID string `json:"match_id"`
}

type IcebreakerResponse struct {
	Icebreaker string `json:"icebreaker"`
	Source     string `json:"source"` // llama | template
	LatencyMs  int64  `json:"latency_ms"`
}

// Metrics ---------------------------------------------------------------

type Metrics struct {
	ActiveUsers      int     `json:"active_users"`
	ActiveMatches    int     `json:"active_matches"`
	MatchesCreated   int64   `json:"matches_created"`
	MatchesExpired   int64   `json:"matches_expired"`
	ScrubsRun        int64   `json:"scrubs_run"`
	PIIBlocked       int64   `json:"pii_blocked"`
	Pipeline         string  `json:"pipeline"`
	AvgMatchLatencyMs float64 `json:"avg_match_latency_ms"`
	P95MatchLatencyMs float64 `json:"p95_match_latency_ms"`
	MatchTTLSeconds  int64   `json:"match_ttl_seconds"`
	MatchRadiusM     float64 `json:"match_radius_m"`
	UptimeSeconds    float64 `json:"uptime_seconds"`
	OllamaModel      string  `json:"ollama_model"`
	OllamaActive     bool    `json:"ollama_active"`
}

// WS protocol -------------------------------------------------------------

type WSClientMsg struct {
	Type   string   `json:"type"` // presence | scan | ping
	UserID string   `json:"user_id,omitempty"`
	Lat    float64  `json:"lat,omitempty"`
	Lng    float64  `json:"lng,omitempty"`
	Interests []string `json:"interests,omitempty"`
}

type WSServerMsg struct {
	Type       string       `json:"type"`
	Match      *Match       `json:"match,omitempty"`
	Radar      *RadarState  `json:"radar,omitempty"`
	LatencyMs  int64        `json:"latency_ms,omitempty"`
	Scrub      *ScrubEvent  `json:"scrub,omitempty"`
	Pipeline   string       `json:"pipeline,omitempty"`
	Message    string       `json:"message,omitempty"`
	TTLSeconds int64        `json:"ttl_seconds,omitempty"`
	ExpiresAt  time.Time    `json:"expires_at,omitempty"`
}