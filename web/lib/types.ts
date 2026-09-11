// Wire types mirrored from backend/internal/model.

export interface Match {
  id: string;
  peer_user_id: string;
  shared_tokens: string[];
  similarity: number;
  distance_m: number;
  icebreaker: string;
  created_at: string;
  expires_at: string;
  ttl_seconds: number;
}

export interface ScrubEvent {
  id: string;
  type: string;
  user_id: string;
  pipeline: string;
  raw_length: number;
  redacted_len?: number;
  tokens?: string[];
  blocked?: string[];
  raw?: string;
  at: string;
}

export interface RadarTarget {
  id: string;
  distance_m: number;
  bearing_deg: number;
  shared_tokens?: string[];
  similarity?: number;
  link: [number[], number[]];
  first_seen: string;
}

export interface RadarState {
  self: number[];
  targets: RadarTarget[];
  radius_m: number;
  sweep_hz: number;
  pipeline_id: string;
}

export interface IcebreakerResponse {
  icebreaker: string;
  source: string;
  latency_ms: number;
}

export interface Metrics {
  active_users: number;
  active_matches: number;
  matches_created: number;
  matches_expired: number;
  scrubs_run: number;
  pii_blocked: number;
  pipeline: string;
  avg_match_latency_ms: number;
  p95_match_latency_ms: number;
  match_ttl_seconds: number;
  match_radius_m: number;
  uptime_seconds: number;
  ollama_model: string;
  ollama_active: boolean;
}

export type ServerMsg =
  | { type: "hello"; message?: string }
  | { type: "pong" }
  | { type: "match"; match: Match; latency_ms?: number }
  | { type: "radar"; radar: RadarState }
  | { type: "pipeline"; scrub: ScrubEvent }
  | { type: "match_ttl"; message?: string; ttl_seconds?: number; expires_at?: string };