package match

import (
	"context"
	"testing"
	"time"

	"github.com/echosync/backend/internal/config"
)

// DoD: 15-Minute Ephemeral TTL — match TTL must be strictly 900s by default.
func TestMatchTTL_Is900Seconds(t *testing.T) {
	ttl := config.Load().Match.TTL
	if ttl != 900*time.Second {
		t.Fatalf("expected 900s TTL, got %s", ttl)
	}
}

func TestMatchID_DeterministicAndOrderIndependent(t *testing.T) {
	a, b := matchID("alice", "bob"), matchID("bob", "alice")
	if a != b {
		t.Fatalf("match id must be order-independent: %s vs %s", a, b)
	}
	if a == "" {
		t.Fatal("empty match id")
	}
}

func TestIntersect(t *testing.T) {
	got := intersect([]string{"RUST PROGRAMMING", "FORMULA 1"}, []string{"FORMULA 1", "CLASSICAL MUSIC"})
	if len(got) != 1 || got[0] != "FORMULA 1" {
		t.Fatalf("unexpected intersection: %v", got)
	}
}

func TestTemplateIcebreaker_OfflineFallback(t *testing.T) {
	g := &Generator{} // no client: forces template path
	prompt, src, _ := g.Generate(context.Background(), []string{"RUST PROGRAMMING"}, 6, "peerA")
	if src != "template" {
		t.Fatalf("expected template source, got %s", src)
	}
	if prompt == "" || !isASCII(prompt) {
		t.Fatalf("template prompt invalid: %q", prompt)
	}
	_ = g
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}