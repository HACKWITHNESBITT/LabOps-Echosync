package privacy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/echosync/backend/internal/config"
)

func TestGroqRedactor_Scrub(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("bad authorization: %q", got)
		}
		gotBody = readAll(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[\"RUST PROGRAMMING\",\"DATA SCIENCE\"]"}}]}`))
	}))
	defer srv.Close()

	r := NewGroqRedactor(config.GroqConfig{
		APIKey: "test-key", BaseURL: srv.URL, Model: "openai/gpt-oss-120b",
	}, "extract interests")
	if r.Name() != PipelineGroq {
		t.Errorf("wrong name %q", r.Name())
	}
	res, err := r.Scrub(context.Background(),
		"My name is Kim Lopez, I do Rust Programming and Data Science, call me at 4155550199")
	if err != nil {
		t.Fatalf("scrub: %v", err)
	}
	if res.Pipeline != PipelineGroq {
		t.Errorf("pipeline=%q", res.Pipeline)
	}
	if !strings.Contains(gotBody, "openai/gpt-oss-120b") {
		t.Errorf("request did not include model: %s", gotBody)
	}
	if !contains(res.Tokens, "RUST PROGRAMMING") {
		t.Errorf("tokens=%v", res.Tokens)
	}
}

func TestGroqRedactor_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	r := NewGroqRedactor(config.GroqConfig{APIKey: "bad", BaseURL: srv.URL, Model: "openai/gpt-oss-120b"}, "")
	if _, err := r.Scrub(context.Background(), "hi"); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestParseTokenJSON(t *testing.T) {
	cases := []struct{ in string; want []string }{
		{`["A","B"]`, []string{"A", "B"}},
		{`{"tokens":["A"]}`, []string{"A"}},
		{"```json\n[\"A\"]\n```", []string{"A"}},
	}
	for _, c := range cases {
		got := parseTokenJSON(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parseTokenJSON(%q)=%v", c.in, got)
		}
	}
}

func readAll(r *http.Request) string {
	body, _ := io.ReadAll(r.Body)
	return string(body)
}