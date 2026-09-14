#!/usr/bin/env bash
# EchoSync end-to-end demo
#
#   1. Privacy firewall: scrubs PII from an ambient transcript, returns only
#      interest tokens.
#   2. Proximity matching: two nearby users who share interests get a match
#      persisted with a Redis-native 15-minute TTL.
#   3. Icebreaker: AI-generated (or template) conversation prompt.
#   4. Latency: reports the proximity->match latency budget (< 500ms DoD).
#
# Prereqs: backend running on $BASE, docker compose infra up.
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8080}"
PINK='\033[38;5;211m'; BOLD='\033[1m'; DIM='\033[2m'; RESET='\033[0m'
step()  { printf "${PINK}${BOLD}── %s${RESET}\n" "$*"; }
info()  { printf "${DIM}%s${RESET}\n" "$*"; }
ok()    { printf "  ${BOLD}✓${RESET} %s\n" "$*"; }

health() { curl -s "$BASE/health" | jq -e '.status == "ok"' >/dev/null; }

step "0 · Health check"
if ! health; then
  echo "  backend not reachable at $BASE — run: make dev (or docker compose up --build)"
  exit 1
fi
ok "backend healthy at $BASE"

step "1 · Privacy Firewall (PII scrub → tokens only)"
TRANSCRIPT="Hey my name is Kim Lopez, my email is kim.lopez@example.com and my phone is +1 (415) 555-0199. \
We were just talking about RUST PROGRAMMING and Formula 1, I'm super into machine learning too."

SCRUB=$(curl -s -X POST "$BASE/v1/scrub" \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"demo_a\",\"transcript\":\"$TRANSCRIPT\"}")

echo "$SCRUB" | jq '{pipeline, blocked_pii, tokens, scrub_latency_ms, embed_latency_ms}'
TOKENS=$(echo "$SCRUB" | jq -r '.tokens | join(",")')

if echo "$TOKENS" | grep -qiE 'kim|lopez|example@|415-?555|555-?0199'; then
  echo "  ✗ PII LEAKED into tokens: $TOKENS" >&2
  exit 1
fi
ok "no PII survived into tokens → tokens: $TOKENS"

step "2 · Proximity matching (15 m geofence) + 15-minute TTL"
# Two users ~8m apart, overlapping interests.
curl -s -X POST "$BASE/v1/presence" -H 'Content-Type: application/json' \
  -d '{"user_id":"demo_a","lat":37.7749295,"lng":-122.4194155,
       "interests":["RUST PROGRAMMING","FORMULA 1","MACHINE LEARNING","HIKING"]}' >/dev/null
A=$(curl -s -X POST "$BASE/v1/presence" -H 'Content-Type: application/json' \
  -d '{"user_id":"usr_7x2","lat":37.7749295,"lng":-122.4194155,
       "interests":["RUST PROGRAMMING","FORMULA 1","HIKING","PHOTOGRAPHY"]}')

echo "$A" | jq '{user_id, processed_ms, matches: [.matches[] | {id, peer_user_id, shared_tokens, similarity, distance_m, ttl_seconds, icebreaker_source: (.icebreaker[0:40])}]}'

NMATCH=$(echo "$A" | jq '.matches | length')
if [ "$NMATCH" -lt 1 ]; then
  echo "  ✗ expected >=1 match between demo_a and usr_7x2" >&2
  exit 1
fi
MATCH_ID=$(echo "$A" | jq -r '.matches[0].id')
TTL=$(echo "$A" | jq -r '.matches[0].ttl_seconds')
ok "match id=$MATCH_ID created with TTL=${TTL}s (Redis EXPIRE)"

step "3 · Ephemeral feed + AI icebreaker"
MATCHES=$(curl -s "$BASE/v1/matches/demo_a")
echo "$MATCHES" | jq '.matches[0] | {peer_user_id, shared_tokens, similarity, distance_m, ttl_seconds}'
ROUND=$(curl -s -X POST "$BASE/v1/icebreaker" -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"demo_a\",\"match_id\":\"$MATCH_ID\"}")
echo "  → $(echo "$ROUND" | jq -r .icebreaker)  (source: $(echo "$ROUND" | jq -r .source), $(echo "$ROUND" | jq -r .latency_ms)ms)"

step "4 · Command-center metrics (latency budget < 500ms)"
M=$(curl -s "$BASE/metrics")
echo "$M" | jq '{active_users, active_matches, matches_created, avg_match_latency_ms, p95_match_latency_ms, match_ttl_seconds, match_radius_m, pipeline, ollama_model}'
LAT=$(echo "$M" | jq -r '.p95_match_latency_ms')
if [ -n "$LAT" ] && [ "$(echo "$LAT > 500" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
  echo "  ⚠ p95 latency above 500ms budget ($LAT ms)"
else
  ok "end-to-end proximity→alert latency $LAT ms (DoD < 500ms)"
fi

step "5 · Ephemeral feed (radar + TTL)"
curl -s "$BASE/v1/matches/demo_a" | jq '.matches[0] | {ttl_seconds}' 
echo
printf "${PINK}${BOLD}Demo complete ✓ — watch the command center dashboards / real WS match events for the radial links + haptics trigger.${RESET}\n"