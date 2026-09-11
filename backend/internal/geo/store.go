// Package geo wraps Redis for the spatial proximity engine:
//   - Redis GEO index of live users within a configurable radius
//   - Redis-native 15-minute ephemeral TTL for match records
package geo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/echosync/backend/internal/config"
)

const (
	geoKey    = "echosync:users:geo"
	tsKey     = "echosync:users:ts"
	matchKey  = "echosync:match:%s"

	maxLatencySamples = 200
)

// MatchRecord is the ephemeral match payload persisted to Redis. It is
// automatically purged by Redis when the TTL (default 15min) elapses.
type MatchRecord struct {
	ID           string    `json:"id"`
	UserA        string    `json:"user_a"`
	UserB        string    `json:"user_b"`
	SharedTokens []string  `json:"shared_tokens"`
	Similarity   float64   `json:"similarity"`
	DistanceM    float64   `json:"distance_m"`
	Icebreaker   string    `json:"icebreaker"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Store is the Redis-backed spatial + ephemeral store.
type Store struct {
	rdb       *redis.Client
	ttl       time.Duration
	latencies []time.Duration
}

func New(ctx context.Context, cfg config.RedisConfig, ttl time.Duration) (*Store, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("geo store: redis ping: %w", err)
	}
	slog.Info("geo store connected", "addr", cfg.Addr, "ttl", ttl)
	return &Store{rdb: rdb, ttl: ttl, latencies: make([]time.Duration, 0, maxLatencySamples)}, nil
}

// UpsertPosition records a user's live location and last-seen timestamp.
func (s *Store) UpsertPosition(ctx context.Context, userID string, lat, lng float64) error {
	pipe := s.rdb.TxPipeline()
	pipe.GeoAdd(ctx, geoKey, &redis.GeoLocation{Longitude: lng, Latitude: lat, Name: userID})
	pipe.ZAdd(ctx, tsKey, redis.Z{Score: float64(time.Now().Unix()), Member: userID})
	_, err := pipe.Exec(ctx)
	return err
}

// Nearby returns candidate user IDs within radiusM of the given user.
func (s *Store) Nearby(ctx context.Context, userID string, radiusM float64) ([]string, error) {
	res, err := s.rdb.GeoSearch(ctx, geoKey, &redis.GeoSearchQuery{
		Member:     userID,
		Radius:     radiusM,
		RadiusUnit: "m",
		Sort:       "ASC",
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("geo search: %w", err)
	}
	return res, nil
}

// DistanceBetween returns the great-circle distance (m) between two users.
func (s *Store) DistanceBetween(ctx context.Context, a, b string) (float64, error) {
	res, err := s.rdb.GeoDist(ctx, geoKey, a, b, "m").Result()
	if err != nil {
		return 0, fmt.Errorf("geo dist: %w", err)
	}
	return res, nil
}

// BearingBetween returns the compass bearing from a toward b.
func (s *Store) BearingBetween(ctx context.Context, a, b string) (float64, error) {
	pos, err := s.rdb.GeoPos(ctx, geoKey, a, b).Result()
	if err != nil {
		return 0, err
	}
	if len(pos) < 2 || pos[0] == nil || pos[1] == nil {
		return 0, errors.New("geo pos unavailable")
	}
	lat1, lon1 := deg2rad(pos[0].Latitude), deg2rad(pos[0].Longitude)
	lat2, lon2 := deg2rad(pos[1].Latitude), deg2rad(pos[1].Longitude)
	y := math.Sin(lon2-lon1) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(lon2-lon1)
	bearing := math.Atan2(y, x) * 180 / math.Pi
	return math.Mod(bearing+360, 360), nil
}

// ActiveUserCount returns the number of live geo-indexed users.
func (s *Store) ActiveUserCount(ctx context.Context) (int, error) {
	n, err := s.rdb.ZCard(ctx, geoKey).Result()
	return int(n), err
}

// ErrNotRegistered is returned when removing a user that has no geo entry.
var ErrNotRegistered = errors.New("user not registered in geo index")

// RemoveUser clears a leaving user from the geo index.
func (s *Store) RemoveUser(ctx context.Context, userID string) error {
	n, err := s.rdb.ZRem(ctx, geoKey, userID).Result()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotRegistered
	}
	return s.rdb.ZRem(ctx, tsKey, userID).Err()
}

// CreateMatch persists an ephemeral match record with a Redis-native TTL.
// Redis expunges it automatically once the TTL expires (default 15 minutes),
// which enforces the "self-destructing match" contract without a janitor.
// CreatedAt is treated as the proximity-detection timestamp so the persisted
// latency can be measured end-to-end.
func (s *Store) CreateMatch(ctx context.Context, rec MatchRecord) error {
	if rec.ExpiresAt.IsZero() {
		rec.ExpiresAt = time.Now().Add(s.ttl)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	ttl := time.Until(rec.ExpiresAt)
	if ttl <= 0 {
		return errors.New("cannot create match with non-positive TTL")
	}
	if err := s.rdb.Set(ctx, fmt.Sprintf(matchKey, rec.ID), data, ttl).Err(); err != nil {
		return fmt.Errorf("redis set match: %w", err)
	}
	if !rec.CreatedAt.IsZero() {
		s.latencies = appendLatency(s.latencies, time.Since(rec.CreatedAt))
	}
	return nil
}

// GetMatch fetches a live ephemeral match.
func (s *Store) GetMatch(ctx context.Context, id string) (*MatchRecord, error) {
	data, err := s.rdb.Get(ctx, fmt.Sprintf(matchKey, id)).Result()
	if err != nil {
		return nil, err
	}
	var rec MatchRecord
	if err := json.Unmarshal([]byte(data), &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// MatchesFor lists the live ephemeral matches involving user.
func (s *Store) MatchesFor(ctx context.Context, user string) ([]MatchRecord, error) {
	var out []MatchRecord
	iter := s.rdb.Scan(ctx, 0, fmt.Sprintf(matchKey, "*"), 100).Iterator()
	for iter.Next(ctx) {
		data, err := s.rdb.Get(ctx, iter.Val()).Result()
		if err != nil {
			continue
		}
		var rec MatchRecord
		if json.Unmarshal([]byte(data), &rec) != nil {
			continue
		}
		if rec.UserA == user || rec.UserB == user {
			out = append(out, rec)
		}
	}
	return out, iter.Err()
}

// ActiveMatchCount returns the number of live ephemeral match keys.
func (s *Store) ActiveMatchCount(ctx context.Context) (int, error) {
	n := 0
	iter := s.rdb.Scan(ctx, 0, fmt.Sprintf(matchKey, "*"), 100).Iterator()
	for iter.Next(ctx) {
		n++
	}
	return n, iter.Err()
}

// MatchTTL returns the remaining TTL for a given match.
func (s *Store) MatchTTL(ctx context.Context, id string) (time.Duration, error) {
	return s.rdb.TTL(ctx, fmt.Sprintf(matchKey, id)).Result()
}

// LatencyStats exposes the rolling proximity->match latency distribution.
// P95 is interpolated from a ring buffer (up to 200 recent samples).
func (s *Store) LatencyStats() (avgMs, p95Ms float64) {
	n := len(s.latencies)
	if n == 0 {
		return 0, 0
	}
	var sum time.Duration
	for _, d := range s.latencies {
		sum += d
	}
	sorted := make([]time.Duration, n)
	copy(sorted, s.latencies)
	for i := 1; i < n; i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	avgMs = float64(sum) / float64(n) / float64(time.Millisecond)
	idx := int(float64(n-1) * 0.95)
	p95Ms = float64(sorted[idx]) / float64(time.Millisecond)
	return avgMs, p95Ms
}

func (s *Store) Close() error { return s.rdb.Close() }

func appendLatency(samples []time.Duration, d time.Duration) []time.Duration {
	if len(samples) < maxLatencySamples {
		return append(samples, d)
	}
	copy(samples, samples[1:])
	samples[len(samples)-1] = d
	return samples
}

func deg2rad(d float64) float64 { return d * math.Pi / 180 }