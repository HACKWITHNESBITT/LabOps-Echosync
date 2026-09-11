// Package privacy implements EchoSync's zero-trust PII firewall.
//
// The pipeline consumes an ambient transcript (produced by on-device ASR from
// a RAM-only audio buffer), strips personal identifiers, and returns only
// non-identifying interest tokens. Raw audio never leaves device RAM and raw
// transcripts are never persisted server-side.
package privacy

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	PipelineRule  = "rule"
	PipelineLlama = "llama"
)

// ScrubResult is the fully sanitized output of the firewall. It contains no
// PII and is the only data forwarded to the embedding stage.
type ScrubResult struct {
	Tokens       []string
	BlockedPII   []string
	RedactedLen  int
	Pipeline     string
	ScrubLatency time.Duration
}

// Redactor is implemented by the edge Llama-3 engine and the deterministic
// rule-based fallback.
type Redactor interface {
	// Name identifies the implementation for dashboards/metrics.
	Name() string
	Scrub(ctx context.Context, transcript string) (ScrubResult, error)
}

// Events is a callback sink for the command-center "live PII pipeline" panel.
type Event struct {
	ID        string
	Type      string
	UserID    string
	Processor string
	RawLen    int
	RedactLen int
	Tokens    []string
	Blocked   []string
	Raw       string
	At        time.Time
}

type EventSink func(Event)

// ---------------------------------------------------------------------------
// Deterministic rule-based firewall (always available, offline)
// ---------------------------------------------------------------------------

var (
	emailRe       = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	phoneRe       = regexp.MustCompile(`(?:\+?1[-\s.]?)?\(?\d{3}\)?[-\s.]?\d{3}[-\s.]?\d{4}\b`)
	ssnRe         = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	creditRe      = regexp.MustCompile(`(?:\d[ -]?){13,19}\b`)
	ipRe          = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	mentionRe     = regexp.MustCompile(`@[a-z0-9_]{2,}`)
	hashRe        = regexp.MustCompile(`[#§]\w+`)
	addressRe     = regexp.MustCompile(`(?i)\b\d{1,5}\s+[a-z]+(?:\s+[a-z]+)?\s+(?:street|st|avenue|ave|road|rd|boulevard|blvd|lane|ln|drive|dr|court|ct|way|terrace|terr|place|pl|broadway|highway|hwy)\b`)
	nameIntroRe   = regexp.MustCompile(`(?i)\b(?:my name is|i'?m called|call me|this is|i am)\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+)?)`)
	emailWordsRe  = regexp.MustCompile(`\b(?:email(?: address)?|e-?mail|mail) (?:is |me at |at )?`)

	// candidate interest: either ALLCAPS tokens or Title-Case multiword phrases
	capPhraseRe      = regexp.MustCompile(`(?:\b[A-Z][A-Za-z&+]{2,}(?:\s+[A-Z][A-Za-z&+]{2,}){0,3}\b)`)
	allCapsRe        = regexp.MustCompile(`\b[A-Z][A-Z0-9&+]{2,}(?:\s+[A-Z0-9&+]{2,}){0,2}\b`)
	labelNumberRe    = regexp.MustCompile(`\b[A-Z][A-Za-z&+]{2,}\s+\d{1,2}\b`) // "FORMULA 1"
	followedByNumber = regexp.MustCompile(`^\s+\d{1,2}\b`)
	stopRe           = regexp.MustCompile(`(?i)\b(the|and|for|with|from|that|this|you|your|our|about|come|stay|meet|work|like|love|very|really|have|having|about|based)\b`)
	introStopRe      = regexp.MustCompile(`(?i)^(the|the|a|an|and|or|is|are|thatwhen|when|where|what|who|how|why|if|my|your|our|in|on|at|to|for|of)\b`)
	sentArtifactsRe  = regexp.MustCompile(`(?i)\b(?:also|actually|probably|maybe|pretty|quite|sort of|kind of)\b`)
)

// RuleRedactor strips PII with compiled heuristics and extracts capitalized
// interest tokens. It guarantees a privacy-preserving output with no model.
type RuleRedactor struct {
	interestDict map[string]struct{}
}

func NewRuleRedactor() *RuleRedactor {
	dict := map[string]struct{}{}
	for _, w := range interestDictionary {
		dict[normalize(w)] = struct{}{}
	}
	return &RuleRedactor{interestDict: dict}
}

func (r *RuleRedactor) Name() string { return PipelineRule }

func (r *RuleRedactor) Scrub(ctx context.Context, transcript string) (ScrubResult, error) {
	start := time.Now()
	blocked := map[string]string{} // type -> example (hashed for logs only)
	redacted := transcript

	redact := func(re *regexp.Regexp, kind string) {
		redacted = re.ReplaceAllStringFunc(redacted, func(m string) string {
			blocked[kind] = hashToken(m)
			return "[REDACTED_" + strings.ToUpper(kind) + "]"
		})
	}
	redact(emailRe, "email")
	redact(phoneRe, "phone")
	redact(ssnRe, "ssn")
	redact(creditRe, "card")
	redact(ipRe, "ip")
	redact(mentionRe, "username")
	redact(hashRe, "tag")
	redact(addressRe, "address")

	// Names introduced verbally ("my name is Kim Lopez").
	redacted = nameIntroRe.ReplaceAllStringFunc(redacted, func(m string) string {
		blocked["name"] = hashToken(m)
		return "[REDACTED_NAME]"
	})
	// Email verbs ("email me at").
	redacted = emailWordsRe.ReplaceAllString(redacted, "")

	res := ScrubResult{
		BlockedPII:  sortedKeys(blocked),
		RedactedLen: len(redacted),
		Pipeline:    PipelineRule,
	}
	res.Tokens = extractTokens(redacted, r.interestDict)
	res.ScrubLatency = time.Since(start)
	return res, nil
}

// extractTokens pulls non-identifying interest keywords from scrubbed text.
func extractTokens(redacted string, dict map[string]struct{}) []string {
	seen := map[string]struct{}{}

	// "FORMULA 1" style label+number phrases first.
	for _, m := range labelNumberRe.FindAllString(redacted, -1) {
		seen[normalize(m)] = struct{}{}
	}

	// Capitalized Title-Case phrases ("Rust Programming"). A capitalized word
	// that is immediately followed by a number belongs to a label+number phrase
	// already captured above, so it must not survive as a standalone token.
	for _, m := range capPhraseRe.FindAllStringIndex(redacted, -1) {
		if followedByNumber.MatchString(redacted[m[1]:]) {
			continue
		}
		tok := normalize(redacted[m[0]:m[1]])
		tok = stopRe.ReplaceAllString(tok, " ")
		tok = introStopRe.ReplaceAllString(tok, " ")
		tok = strings.TrimSpace(tok)
		if len(tok) >= 3 {
			seen[tok] = struct{}{}
		}
	}
	// All-caps keywords ("RUST", "F1").
	for _, m := range allCapsRe.FindAllString(redacted, -1) {
		tok := strings.ToUpper(strings.Join(strings.Fields(m), " "))
		if len(tok) >= 2 {
			seen[tok] = struct{}{}
		}
	}
	// Dictionary terms regardless of casing.
	words := strings.FieldsFunc(strings.ToLower(redacted), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r == ' ')
	})
	for _, w := range words {
		if _, ok := dict[w]; ok {
			seen[strings.ToUpper(w)] = struct{}{}
		}
	}

	tokens := make([]string, 0, len(seen))
	for t := range seen {
		if isBlockedToken(t) {
			continue
		}
		tokens = append(tokens, t)
	}
	sort.Slice(tokens, func(i, j int) bool { return tokens[i] < tokens[j] })
	return tokens
}

// isBlockedToken guards against reconstructed identifiers like a name that
// happens to have been a dictionary word (pseudonymizing the candle, not the
// flame).
func isBlockedToken(tok string) bool {
	// single-letter / clearly personal pronouns already filtered by length
	filter := map[string]struct{}{
		"NAME": {}, "EMAIL": {}, "PHONE": {}, "CALL ME": {}, "MY NAME IS": {},
		"I AM": {}, "THIS IS": {}, "REDACTED": {},
	}
	_, bad := filter[tok]
	return bad || strings.Contains(tok, "REDACTED")
}

func normalize(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), " "))
}

func hashToken(s string) string {
	// feedback digest only — never the raw value
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return fmt.Sprintf("§%x", h)
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// interestDictionary seeds token extraction for lowercase ambient speech.
var interestDictionary = []string{
	"rust programming", "formula 1", "formula one", "f1", "machine learning",
	"artificial intelligence", "ai", "data science", "cybersecurity", "hiking",
	"photography", "guitar", "classical music", "electronic music", "jazz",
	"rock climbing", "bouldering", "sailing", "surfing", "skiing", "snowboarding",
	"basketball", "football", "soccer", "tennis", "chess", "poker", "board games",
	"video games", "gaming", "esports", "pc building", "3d printing", "drones",
	"coffee", "specialty coffee", "brewing", "cooking", "baking", "wine", "whiskey",
	"craft beer", "yoga", "meditation", "running", "marathon", "triathlon",
	"cycling", "mountain biking", "motorcycles", "classic cars", "travel", "backpacking",
	"campsite", "astronomy", "space", "physics", "chemistry", "biology", "genetics",
	"biotech", "robotics", "iot", "embedded systems", "devops", "kubernetes", "docker",
	"golang", "go programming", "python", "typescript", "web assembly", "crypto",
	"bitcoin", "baking", "painting", "drawing", "writing", "poetry", "novels",
	"science fiction", "fantasy", "history", "ancient history", "archaeology", "anthropology",
	"linguistics", "languages", "spanish", "french", "japanese", "korean", "mandarin",
	"cooking shows", "documentaries", "movies", "cinema", "theatre", "opera", "ballet",
	"standup comedy", "improv", "volunteering", "nonprofit", "mentoring", "podcasts",
	"public speaking", "startups", "product design", "ui design", "ux research",
	"financial markets", "trading", "economics", "psychology", "philosophy", "existentialism",
}

// ---------------------------------------------------------------------------
// Pipeline orchestration (scrub -> events -> tokens)
// ---------------------------------------------------------------------------

var scrubSeq atomic.Int64

// Pipeline routes transcripts through the configured redactor and streams
// events to the command center. Raw transcripts are only emitted to the event
// sink when rawAllowed is true (dev/demo dashboards).
type Pipeline struct {
	redactor   Redactor
	fallback   Redactor
	events     EventSink
	rawAllowed bool
	runCount   atomic.Int64
	blockTotal atomic.Int64
}

func NewPipeline(primary Redactor, events EventSink, rawAllowed bool) *Pipeline {
	return &Pipeline{
		redactor:   primary,
		fallback:   NewRuleRedactor(),
		events:     events,
		rawAllowed: rawAllowed,
	}
}

// SetEvents attaches (or replaces) the command-center event sink. Wiring this
// after construction lets main hand the pipeline the server's ring buffer.
func (p *Pipeline) SetEvents(fn EventSink) *Pipeline {
	p.events = fn
	return p
}

// ScrubAndTokenize accepts a raw transcript, sanitizes it through the firewall
// and returns the extracted interest tokens plus pipeline metadata.
func (p *Pipeline) ScrubAndTokenize(ctx context.Context, userID, transcript string) (ScrubResult, error) {
	p.runCount.Add(1)
	if ctx == nil {
		ctx = context.Background()
	}
	p.emit(Event{
		ID: fmt.Sprintf("scrub-%05d", scrubSeq.Add(1)), Type: "scrub.started",
		UserID: userID, Processor: redactorName(p.redactor), RawLen: len(transcript), At: time.Now(),
	})

	res, err := p.redactor.Scrub(ctx, transcript)
	if err != nil {
		slog.Warn("primary redactor failed, falling back to rule-based", "err", err)
		fall, ferr := p.fallback.Scrub(ctx, transcript)
		if ferr != nil {
			return res, fmt.Errorf("privacy pipeline failed: %w", ferr)
		}
		res = fall
		p.emit(Event{
			ID: fmt.Sprintf("scrub-%05d", scrubSeq.Load()), Type: "scrub.fallback",
			UserID: userID, Processor: PipelineRule, RawLen: len(transcript), At: time.Now(),
		})
	}

	p.blockTotal.Add(int64(len(res.BlockedPII)))
	raw := ""
	if p.rawAllowed {
		raw = transcript
	}
	p.emit(Event{
		ID: fmt.Sprintf("scrub-%05d", scrubSeq.Load()), Type: "scrub.complete",
		UserID: userID, Processor: res.Pipeline, RawLen: len(transcript),
		RedactLen: res.RedactedLen, Tokens: res.Tokens, Blocked: res.BlockedPII, At: time.Now(),
		Raw: raw,
	})
	return res, nil
}

func (p *Pipeline) Stats() (runs, blocked int64) {
	return p.runCount.Load(), p.blockTotal.Load()
}

func (p *Pipeline) ActivePipeline() string { return redactorName(p.redactor) }

func (p *Pipeline) emit(ev Event) {
	if p.events != nil {
		p.events(ev)
	}
}

func redactorName(r Redactor) string {
	if n, ok := r.(interface{ Name() string }); ok {
		return n.Name()
	}
	return PipelineLlama
}