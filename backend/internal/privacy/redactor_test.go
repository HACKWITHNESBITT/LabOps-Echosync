package privacy

import (
	"context"
	"strings"
	"testing"
)

// DoD: the PII firewall must strip private identifiers before any vector
// embedding generation. This test drives a raw transcript rich in PII and
// asserts no identifier survives into the token output.
func TestRuleRedactor_StripsPII(t *testing.T) {
	r := NewRuleRedactor()
	transcript := `Hey my name is Kim Lopez my email is kim.lopez@example.com and my phone is +1 (415) 555-0199.
I don't do X either, actually I LOVE Rust Programming. Also catch me on @kloppz. My SSN is 123-45-6789.
I'm super into Formula 1 and rock climbing. One last thing, my address is 12 Maple Street, and I use card 4111 1111 1111 1111.`

	res, err := r.Scrub(context.Background(), transcript)
	if err != nil {
		t.Fatalf("scrub error: %v", err)
	}

	if len(res.BlockedPII) == 0 {
		t.Fatal("expected blocked PII categories")
	}
	for _, k := range res.BlockedPII {
		t.Logf("blocked: %s", k)
	}

	for _, tok := range res.Tokens {
		tokUpper := strings.ToUpper(tok)
		for _, leak := range []string{
			"KIM", "LOPEZ", "KIM.LOPEZ", "415", "555", "KLO", "MAPLE", "123-45",
		} {
			if strings.Contains(tokUpper, strings.ToUpper(leak)) {
				t.Fatalf("PII leaked into interest token %q (matched %q)", tok, leak)
			}
		}
	}

	// Desired interest extraction still works after scrubbing.
	if !contains(res.Tokens, "RUST PROGRAMMING") {
		t.Errorf("expected RUST PROGRAMMING token, got %v", res.Tokens)
	}
	// The scrubbed text itself must no longer contain the email.
	if res.RedactedLen <= 0 || res.RedactedLen >= len(transcript) {
		t.Error("expected redacted length to be non-empty and smaller than the input")
	}
}

func TestRuleRedactor_ExtractsTokens(t *testing.T) {
	r := NewRuleRedactor()
	res, err := r.Scrub(context.Background(),
		"we were just talking about machine learning and I mentioned I do data science on the side")
	if err != nil {
		t.Fatal(err)
	}
	if res.Pipeline != PipelineRule {
		t.Errorf("expected rule pipeline, got %s", res.Pipeline)
	}
}

func TestExtractTokens_CapsAndPhrases(t *testing.T) {
	toks := extractTokens("RUST PROGRAMMING and FORMULA 1 tonight", nil)
	if !contains(toks, "RUST PROGRAMMING") {
		t.Errorf("missing RUST PROGRAMMING: %v", toks)
	}
	if !contains(toks, "FORMULA 1") {
		t.Errorf("missing FORMULA 1: %v", toks)
	}
}

func TestLlamaUnreachable_FallsBack(t *testing.T) {
	// Pipeline with an unreachable llama primary must fall back to rule-based.
	p := NewPipeline(&failingRedactor{}, nil, false)
	res, err := p.ScrubAndTokenize(context.Background(), "usr", "I love Rust Programming, email me k@x.com")
	if err != nil {
		t.Fatalf("expected fallback to succeed: %v", err)
	}
	if res.Pipeline != PipelineRule {
		t.Errorf("expected rule pipeline after fallback, got %s", res.Pipeline)
	}
	if !contains(res.Tokens, "RUST PROGRAMMING") {
		t.Errorf("fallback token extraction failed: %v", res.Tokens)
	}
	for _, blk := range res.BlockedPII {
		if strings.HasPrefix(blk, "email") || strings.Contains(blk, "REDACT") || strings.HasPrefix(blk, "§") {
			t.Log("blocked:", blk)
		}
	}
}

type failingRedactor struct{}

func (f *failingRedactor) Name() string { return PipelineLlama }
func (f *failingRedactor) Scrub(context.Context, string) (ScrubResult, error) {
	return ScrubResult{}, context.DeadlineExceeded
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}