package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/echosync/backend/internal/audio"
	"github.com/echosync/backend/internal/model"

	"github.com/gorilla/websocket"
)

// audioUpgrader accepts LAN demo clients (Flutter web + curl).
var audioUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// audioOneShot accepts a full audio clip (WAV or raw PCM), transcribes it
// through Speechmatics, scrubs PII via the firewall and registers presence so
// the matching engine can evaluate the 15-meter geofence.
//
//	POST /v1/audio?user_id=X&lat=37.77&lng=-122.41
//	Content-Type: audio/wav | audio/raw (+ X-Sample-Rate)
//	body: audio bytes
func (s *Server) audioOneShot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if s.audio == nil {
		writeErr(w, http.StatusServiceUnavailable, "audio transcription not configured")
		return
	}
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if userID == "" {
		writeErr(w, http.StatusBadRequest, "user_id required")
		return
	}
	lat, lng := queryCoord(r, "lat", 37.7749295), queryCoord(r, "lng", -122.4194155)

	audioBytes, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid audio body")
		return
	}
	contentType := r.Header.Get("Content-Type")

	start := time.Now()
	transcript, err := s.audio.Transcribe(r.Context(), contentType, audioBytes)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "transcription failed: "+err.Error())
		return
	}
	if strings.TrimSpace(transcript) == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id": userID, "transcript": "", "tokens": []string{}, "matches": []any{},
			"total_ms": time.Since(start).Milliseconds(),
		})
		return
	}

	res, err := s.privacy.ScrubAndTokenize(r.Context(), userID, transcript)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "privacy pipeline failed: "+err.Error())
		return
	}
	var created []model.Match
	if len(res.Tokens) > 0 {
		created, err = s.engine.ProcessPresence(r.Context(), model.Presence{
			UserID: userID, Lat: lat, Lng: lng, Interests: res.Tokens,
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "presence processing failed: "+err.Error())
			return
		}
	}
	if created == nil {
		created = []model.Match{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":     userID,
		"transcript":  transcript,
		"pipeline":    res.Pipeline,
		"tokens":      res.Tokens,
		"blocked_pii": res.BlockedPII,
		"matches":     created,
		"total_ms":    time.Since(start).Milliseconds(),
	})
}

// audioStream is the long-lived mic transport. The mobile client streams raw
// PCM over this socket; the backend forwards frames to Speechmatics and, as
// finals arrive, runs them through the privacy firewall + matching engine.
//
//	GET /v1/audio/stream?user_id=X
//	text:   {"type":"start","lat":37.77,"lng":-122.41}   (optional)
//	binary: pcm_s16le sample frames
//	text:   {"type":"flush"} | {"type":"stop"}
func (s *Server) audioStream(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if userID == "" {
		writeErr(w, http.StatusBadRequest, "user_id required")
		return
	}
	if s.audio == nil {
		writeErr(w, http.StatusServiceUnavailable, "audio streaming not configured")
		return
	}
	conn, err := audioUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	lat, lng := queryCoord(r, "lat", 37.7749295), queryCoord(r, "lng", -122.4194155)
	coords := func() (float64, float64) { return lat, lng }
	var sess *audio.Session
	defer func() {
		if sess != nil {
			_ = sess.Close()
		}
	}()

	// Debounce consecutive live finals into one background utterance so burst
	// ASR doesn't hammer the cloud firewall once per word-final.
	utter := newUtteranceBatcher(func(text string) {
		s.handleFinalTranscript(r.Context(), userID, text, coords)
	})
	defer utter.flush()

	openSession := func() bool {
		ns, err := s.audio.Open(r.Context(), func(final string) {
			utter.append(final)
			_ = conn.WriteJSON(map[string]any{"type": "final", "transcript": final})
		}, func(partial string) {
			_ = conn.WriteJSON(map[string]any{"type": "partial", "transcript": partial})
		})
		if err != nil {
			slog.Warn("speechmatics open failed", "user", userID, "err", err)
			_ = conn.WriteJSON(map[string]any{"type": "error", "reason": "speechmatics unavailable"})
			return false
		}
		sess = ns
		return true
	}

	if !openSession() {
		return
	}

	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return // client gone; session closed by defer
		}
		if mt == websocket.BinaryMessage {
			if len(data) > 0 {
				if err := sess.SendAudio(r.Context(), data); err != nil {
					slog.Warn("audio forward failed", "user", userID, "err", err)
				}
			}
			continue
		}
		var cmd struct {
			Type string  `json:"type"`
			Lat  float64 `json:"lat"`
			Lng  float64 `json:"lng"`
		}
		if err := json.Unmarshal(data, &cmd); err != nil {
			continue
		}
		switch cmd.Type {
		case "start":
			if cmd.Lat != 0 || cmd.Lng != 0 {
				lat, lng = cmd.Lat, cmd.Lng
			}
		case "flush":
			utter.flush()
			if err := sess.Flush(); err != nil {
				slog.Warn("speechmatics flush failed", "user", userID, "err", err)
			}
		case "stop":
			utter.flush()
			return
		default:
			// heartbeat / other client JSON ignored
		}
	}
}

// utteranceBatcher coalesces rapid live finals into a single scrub call.
type utteranceBatcher struct {
	mu      sync.Mutex
	buf     []string
	ttl     time.Duration
	timer   *time.Timer
	onFlush func(text string)
}

func newUtteranceBatcher(onFlush func(string)) *utteranceBatcher {
	return &utteranceBatcher{ttl: 1500 * time.Millisecond, onFlush: onFlush}
}

func (u *utteranceBatcher) append(text string) {
	u.mu.Lock()
	u.buf = append(u.buf, text)
	if u.timer == nil {
		u.timer = time.AfterFunc(u.ttl, u.flush)
	} else {
		u.timer.Reset(u.ttl)
	}
	u.mu.Unlock()
}

func (u *utteranceBatcher) flush() {
	u.mu.Lock()
	if len(u.buf) == 0 {
		u.mu.Unlock()
		return
	}
	text := strings.Join(u.buf, " ")
	u.buf = nil
	if u.timer != nil {
		u.timer.Stop()
		u.timer = nil
	}
	u.mu.Unlock()
	u.onFlush(text)
}

// handleFinalTranscript scrubs one finalized utterance and drives presence.
func (s *Server) handleFinalTranscript(ctx context.Context, userID, transcript string, coords func() (float64, float64)) {
	if strings.TrimSpace(transcript) == "" {
		return
	}
	lat, lng := coords()
	res, err := s.privacy.ScrubAndTokenize(ctx, userID, transcript)
	if err != nil {
		slog.Warn("audio scrub failed", "user", userID, "err", err)
		return
	}
	if len(res.Tokens) == 0 {
		return
	}
	if _, err := s.engine.ProcessPresence(ctx, model.Presence{
		UserID: userID, Lat: lat, Lng: lng, Interests: res.Tokens,
	}); err != nil {
		slog.Warn("audio presence failed", "user", userID, "err", err)
	}
}

func queryCoord(r *http.Request, key string, def float64) float64 {
	if v := r.URL.Query().Get(key); v != "" {
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err == nil {
			return f
		}
	}
	return def
}