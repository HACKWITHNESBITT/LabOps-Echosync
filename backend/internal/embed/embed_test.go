package embed

import (
	"context"
	"math"
	"testing"
)

func TestHashEmbedder_DimAndUnitNorm(t *testing.T) {
	h := &HashEmbedder{}
	v, err := h.Embed(context.Background(), []string{"RUST PROGRAMMING", "FORMULA 1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != Dim {
		t.Fatalf("expected dim %d, got %d", Dim, len(v))
	}
	norm := 0.0
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if math.Abs(norm-1) > 1e-4 {
		t.Fatalf("expected unit vector, got norm %f", norm)
	}
}

func TestHashEmbedder_Deterministic(t *testing.T) {
	h := &HashEmbedder{}
	a, _ := h.Embed(context.Background(), []string{"cooking", "wine"})
	b, _ := h.Embed(context.Background(), []string{"cooking", "wine"})
	if len(a) != len(b) {
		t.Fatal("dim mismatch")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("embedding must be deterministic at index %d", i)
		}
	}
}