// Package config loads EchoSync server configuration from environment
// variables with sane local-development defaults.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr   string
	Redis      RedisConfig
	Postgres   PostgresConfig
	Ollama     OllamaConfig
	Embed      EmbedConfig
	Match      MatchConfig
	PII        PIIConfig
	LogLevel   string
	ShowRaw    bool
	StoreStats bool
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type PostgresConfig struct {
	DSN      string
	PoolMax  int
	Timeout  time.Duration
}

type OllamaConfig struct {
	URL        string
	Model      string
	Timeout    time.Duration
	BaseSystem string
}

type EmbedConfig struct {
	Provider string
	Model    string
}

type MatchConfig struct {
	RadiusM     float64
	TTL         time.Duration
	Similarity  float64
	MinTokens   int
}

type PIIConfig struct {
	Mode          string
	MaxBytes      int
	OllamaScrubPrompt string
}

func Load() Config {
	return Config{
		HTTPAddr: envar("ECHO_HTTP_ADDR", ":8080"),
		Redis: RedisConfig{
			Addr:     envar("ECHO_REDIS_ADDR", "127.0.0.1:6379"),
			Password: os.Getenv("ECHO_REDIS_PASSWORD"),
			DB:       envarInt("ECHO_REDIS_DB", 0),
		},
		Postgres: PostgresConfig{
			DSN:     envar("ECHO_PG_DSN", "postgres://echosync:echosync@127.0.0.1:5544/echosync?sslmode=disable"),
			PoolMax: envarInt("ECHO_PG_POOL_MAX", 10),
			Timeout: 5 * time.Second,
		},
		Ollama: OllamaConfig{
			URL:        strings.TrimSuffix(envar("ECHO_OLLAMA_URL", "http://127.0.0.1:11434"), "/"),
			Model:      envar("ECHO_OLLAMA_MODEL", "llama3:8b"),
			Timeout:    time.Duration(envarInt("ECHO_OLLAMA_TIMEOUT_S", 20)) * time.Second,
			BaseSystem: "You are EchoSync's privacy firewall. You extract non-identifying interest tokens and absolutely never repeat names, emails, phone numbers or other PII.",
		},
		Embed: EmbedConfig{
			Provider: envar("ECHO_EMBED_PROVIDER", "hash"),
			Model:    envar("ECHO_EMBED_MODEL", "nomic-embed-text"),
		},
		Match: MatchConfig{
			RadiusM:    envarFloat("ECHO_MATCH_RADIUS_M", 15),
			TTL:        time.Duration(envarInt("ECHO_MATCH_TTL_S", 900)) * time.Second,
			Similarity: envarFloat("ECHO_MATCH_SIMILARITY", 0.60),
			MinTokens:  envarInt("ECHO_MATCH_TOKENS", 3),
		},
		PII: PIIConfig{
			Mode:     envar("ECHO_PII_MODE", "auto"),
			MaxBytes: envarInt("ECHO_PII_MAX_TRANSCRIPT_BYTES", 4096),
			OllamaScrubPrompt: `You are EchoSync's zero-trust PII firewall running on-device.
Given an ambient speech transcript, remove everything that could identify a person (names, emails, phone numbers, addresses, company employer, usernames, government IDs).
Then output ONLY a JSON array of capitalized interest keywords derived from the remaining text (for example ["RUST PROGRAMMING", "FORMULA 1"]).
Never include raw names or contact details in the output.`,
		},
		LogLevel:   envar("ECHO_LOG_LEVEL", "info"),
		ShowRaw:    envarBool("ECHO_SHOW_RAW_EVENTS", false),
		StoreStats: true,
	}
}

func envar(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envarInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func envarFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return def
}

func envarBool(key string, def bool) bool {
	if v, err := strconv.ParseBool(os.Getenv(key)); err == nil {
		return v
	}
	return def
}