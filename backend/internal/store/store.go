// Package store wraps PostgreSQL + pgvector for interest-vector similarity.
package store

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/echosync/backend/internal/config"
)



// Candidate is a nearby peer plus its pgvector cosine similarity to self.
type Candidate struct {
	UserID     string
	Interests  []string
	Similarity float64
}

// VectorStore persists scrubbed interest tokens + embeddings.
type VectorStore struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, cfg config.PostgresConfig) (*VectorStore, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("pg parse dsn: %w", err)
	}
	poolCfg.MaxConns = int32(cfg.PoolMax)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("pg pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pg ping: %w", err)
	}
	slog.Info("vector store connected", "dsn", maskDSN(cfg.DSN))
	return &VectorStore{pool: pool}, nil
}

// UpsertVector writes a user's interest tokens + embedding (pgvector).
// It issues a setup query ensuring the vector extension/schema exist so the
// store works even when init.sql is behind (covers unit-test dbs).
func (s *VectorStore) UpsertVector(ctx context.Context, userID string, interests []string, embedding []float32) error {
	if _, err := s.pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO interest_vectors (user_id, interests, embedding, last_seen)
		VALUES ($1, $2, $3::vector, now())
		ON CONFLICT (user_id) DO UPDATE SET
			interests = EXCLUDED.interests,
			embedding = EXCLUDED.embedding,
			last_seen = now()`,
		userID, interests, vecString(embedding)); err != nil {
		return fmt.Errorf("pg upsert vector: %w", err)
	}
	return nil
}

// CandidatesWithSimilarity ranks the geo candidates against selfEmbedding
// using the pgvector cosine-distance operator (<=>).
func (s *VectorStore) CandidatesWithSimilarity(ctx context.Context, candidateIDs []string, selfEmbedding []float32) ([]Candidate, error) {
	if len(candidateIDs) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, interests, 1 - (embedding <=> $1::vector) AS similarity
		FROM interest_vectors
		WHERE user_id = ANY($2)`,
		vecString(selfEmbedding), candidateIDs)
	if err != nil {
		return nil, fmt.Errorf("pg similarity query: %w", err)
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.UserID, &c.Interests, &c.Similarity); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// EmbeddingFor fetches a single user's stored embedding (absent -> nil).
func (s *VectorStore) EmbeddingFor(ctx context.Context, userID string) ([]float32, error) {
	var raw string
	err := s.pool.QueryRow(ctx,
		`SELECT embedding::text FROM interest_vectors WHERE user_id = $1`, userID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	return parseVec(raw), nil
}

// CountUsers returns the number of users in the vector index.
func (s *VectorStore) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM interest_vectors`).Scan(&n)
	return n, err
}

// InterestsFor returns a single user's scrubbed interest tokens (absent err).
func (s *VectorStore) InterestsFor(ctx context.Context, userID string) ([]string, error) {
	var interests []string
	err := s.pool.QueryRow(ctx,
		`SELECT interests FROM interest_vectors WHERE user_id = $1`, userID).Scan(&interests)
	if err != nil {
		return nil, err
	}
	return interests, nil
}

func (s *VectorStore) Close() { s.pool.Close() }

// vecString renders []float32 as a pgvector literal like "[a,b,...]".
// The dimension must match the column (384).
func vecString(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(fmt.Sprintf("%g", float64(x)))
	}
	b.WriteByte(']')
	return b.String()
}

func parseVec(s string) []float32 {
	s = strings.Trim(s, "[]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]float32, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
			out = append(out, float32(f))
		}
	}
	return out
}

func maskDSN(dsn string) string {
	// strip password for logs
	idx := strings.Index(dsn, "://")
	if idx < 0 {
		return dsn
	}
	rest := dsn[idx+3:]
	if at := strings.Index(rest, "@"); at >= 0 {
		rest = rest[at+1:]
	}
	return dsn[:idx+3] + "***@" + rest
}

