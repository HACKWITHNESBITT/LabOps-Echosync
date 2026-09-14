// Package audio provides real-time speech-to-text ingestion via the
// Speechmatics streaming WebSocket API. Raw microphone PCM frames are pushed
// into a session and final transcript callbacks feed the privacy pipeline.
package audio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/echosync/backend/internal/config"

	"github.com/gorilla/websocket"
)

// TranscriptHandler receives a surfaced transcript (final or partial) along
// with the accumulated utterance text.
type TranscriptHandler func(transcript string)

// Handler owns the Speechmatics connection settings and the Groq Whisper
// fallback used for one-shot file transcription.
type Handler struct {
	cfg          config.SpeechmaticsConfig
	groqKey      string
	groqBase     string
	whisperModel string
}

func NewHandler(cfg config.SpeechmaticsConfig, groq config.GroqConfig) *Handler {
	return &Handler{
		cfg:          cfg,
		groqKey:      groq.APIKey,
		groqBase:     strings.TrimSuffix(groq.BaseURL, "/"),
		whisperModel: groq.WhisperModel,
	}
}

// Config exposes the active configuration for dashboards.
func (h *Handler) Config() config.SpeechmaticsConfig { return h.cfg }

// Session is one Speechmatics recognition session.
type Session struct {
	conn        *websocket.Conn
	cfg         config.SpeechmaticsConfig
	mu          sync.Mutex
	audioFrames int
	onFinal     TranscriptHandler
	onPartial   TranscriptHandler
	partialSum  string
	finalSum    string
	done        chan struct{}
}

// Done is closed once the server has finished the session (EndOfTranscript)
// or an error/close occurred.
func (s *Session) Done() <-chan struct{} { return s.done }

// Open dials Speechmatics, authenticates with the long-lived API key and
// starts a recognition session at the configured sample rate. The read pump
// runs on a goroutine.
func (h *Handler) Open(ctx context.Context, onFinal, onPartial TranscriptHandler) (*Session, error) {
	return h.openRate(ctx, h.cfg.SampleRate, onFinal, onPartial)
}

func (h *Handler) openRate(ctx context.Context, sampleRate int, onFinal, onPartial TranscriptHandler) (*Session, error) {
	if h.cfg.APIKey == "" {
		return nil, fmt.Errorf("SPEECHMATICS_API_KEY not set")
	}
	sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	header := map[string][]string{
		"Authorization": {"Bearer " + h.cfg.APIKey},
		"User-Agent":    {"echosync-backend/1.0"},
	}
	conn, resp, err := websocket.DefaultDialer.DialContext(sctx, h.cfg.URL, header)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("speechmatics dial (http %d): %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("speechmatics dial: %w", err)
	}

	s := &Session{conn: conn, cfg: h.cfg, onFinal: onFinal, onPartial: onPartial, done: make(chan struct{})}
	start := speechmaticsMessage{Message: "StartRecognition"}
	start.AudioFormat = &audioFormat{
		Type: "raw", Encoding: "pcm_s16le", SampleRate: sampleRate,
	}
	start.TranscriptionConfig = &transcriptionConfig{
		Language: h.cfg.Language, EnablePartials: true, MaxDelay: 1.0,
	}
	if err := conn.WriteJSON(start); err != nil {
		conn.Close()
		return nil, fmt.Errorf("speechmatics start: %w", err)
	}
	go s.readPump()
	return s, nil
}

// SendAudio forwards one binary PCM chunk to the recognizer.
func (s *Session) SendAudio(ctx context.Context, pcm []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audioFrames++
	if err := s.conn.WriteMessage(websocket.BinaryMessage, pcm); err != nil {
		return fmt.Errorf("speechmatics send audio: %w", err)
	}
	return nil
}

// Flush asks the recognizer to finalize the current utterance immediately.
func (s *Session) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteJSON(speechmaticsMessage{Message: "ForceEndOfUtterance"})
}

// Final keeps the last final transcript stored on the session.
func (s *Session) Final() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finalSum
}

// EndOfStream tells the recognizer no more audio is coming; finals are
// emitted in response and the session completes with EndOfTranscript.
func (s *Session) EndOfStream() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteJSON(speechmaticsMessage{Message: "EndOfStream", LastSeqNo: s.audioFrames})
}

// Close tears down the transport.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second))
	return s.conn.Close()
}

func (s *Session) readPump() {
	defer func() {
		select {
		case <-s.done:
		default:
			close(s.done)
		}
		s.conn.Close()
	}()
	for {
		mt, data, err := s.conn.ReadMessage()
		if err != nil {
			slog.Debug("speechmatics read closed", "err", err)
			return
		}
		if mt != websocket.TextMessage {
			continue
		}
		var msg struct {
			Message     string `json:"message"`
			Type        string `json:"type,omitempty"`
			Reason      string `json:"reason,omitempty"`
			LastSeqNo   int    `json:"last_seq_no,omitempty"`
			Metadata    struct {
				Transcript string `json:"transcript"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			slog.Debug("speechmatics non-json message", "raw", truncate(string(data), 200))
			continue
		}
		switch msg.Message {
		case "AddPartialTranscript":
			if msg.Metadata.Transcript != "" {
				s.mu.Lock()
				s.partialSum = msg.Metadata.Transcript
				s.mu.Unlock()
				if s.onPartial != nil {
					s.onPartial(msg.Metadata.Transcript)
				}
			}
		case "AddTranscript":
			if msg.Metadata.Transcript != "" {
				s.mu.Lock()
				s.finalSum = msg.Metadata.Transcript
				s.mu.Unlock()
				if s.onFinal != nil {
					s.onFinal(msg.Metadata.Transcript)
				}
			}
		case "RecognitionStarted", "AudioAdded":
			// informational
		case "EndOfTranscript":
			return // session complete
		case "Info":
			// quota / model quality notice
		case "Error":
			slog.Warn("speechmatics error", "type", msg.Type, "reason", msg.Reason)
			return
		default:
			slog.Debug("speechmatics message", "message", msg.Message)
		}
	}
}

// Transcribe turns one uploaded clip into text. Encoded files (WAV/MP3/OGG)
// go to Groq Whisper for full-length, low-latency results; raw PCM streams
// through a Speechmatics session so sample-rate and framing stay exact.
func (h *Handler) Transcribe(ctx context.Context, contentType string, data []byte) (string, error) {
	if !strings.Contains(contentType, "raw") && (!strings.HasPrefix(contentType, "audio/") || bytes.HasPrefix(data, []byte("RIFF"))) {
		if h.whisperModel != "" && h.groqKey != "" {
			text, err := h.whisper(ctx, data)
			if err == nil && strings.TrimSpace(text) != "" {
				return text, nil
			}
			slog.Warn("whisper transcription failed, falling back to speechmatics", "err", err)
		}
	}
	sr, pcm, err := DecodeAudio(contentType, data)
	if err != nil {
		return "", err
	}
	return h.speechmaticsTranscribe(ctx, sr, pcm)
}

// speechmaticsTranscribe runs a one-shot Speechmatics session over raw PCM.
func (h *Handler) speechmaticsTranscribe(ctx context.Context, sampleRate int, pcm []byte) (string, error) {
	var finals []string
	var mu sync.Mutex
	s, err := h.openRate(ctx, sampleRate, func(t string) {
		mu.Lock()
		finals = append(finals, t)
		mu.Unlock()
	}, nil)
	if err != nil {
		return "", err
	}
	defer s.Close()
	if err := s.SendAudio(ctx, pcm); err != nil {
		return "", err
	}
	if err := s.Flush(); err != nil {
		return "", err
	}
	if err := s.EndOfStream(); err != nil {
		return "", err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	select {
	case <-s.Done():
	case <-waitCtx.Done():
		return "", fmt.Errorf("speechmatics transcription timeout")
	}
	mu.Lock()
	defer mu.Unlock()
	return strings.Join(finals, " "), nil
}

// whisper transcribes an encoded audio file via Groq's OpenAI-compatible
// /audio/transcriptions endpoint (whisper-large-v3-turbo).
func (h *Handler) whisper(ctx context.Context, data []byte) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "sample.wav")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := mw.WriteField("model", h.whisperModel); err != nil {
		return "", err
	}
	if err := mw.WriteField("response_format", "json"); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.groqBase+"/audio/transcriptions", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+h.groqKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whisper http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Text), nil
}

// message types mirror the Speechmatics RT API (PascalCase "message" field).
type speechmaticsMessage struct {
	Message            string               `json:"message"`
	AudioFormat        *audioFormat         `json:"audio_format,omitempty"`
	TranscriptionConfig *transcriptionConfig `json:"transcription_config,omitempty"`
	LastSeqNo          int                  `json:"last_seq_no,omitempty"`
}

type audioFormat struct {
	Type       string `json:"type"`
	Encoding   string `json:"encoding"`
	SampleRate int    `json:"sample_rate"`
}

type transcriptionConfig struct {
	Language       string  `json:"language"`
	EnablePartials bool    `json:"enable_partials"`
	MaxDelay       float64 `json:"max_delay"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}