// Package api wires the EchoSync HTTP + WebSocket surface to the engine.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/echosync/backend/internal/model"
	"github.com/echosync/backend/internal/privacy"
)

const (
	wsPushBuffer = 64
	wsReadLimit  = 16 << 10
)

// dashboardIDs are WS identities reserved for command-center observers, which
// receive pipeline + match events but do not inject presence.
var dashboardIDs = map[string]bool{"dashboard": true, "cmd": true, "root": true}

// client is a single connected mobile app or command-center observer.
type client struct {
	conn   *websocket.Conn
	send   chan []byte
	userID string
	dash   bool
}

// Hub multiplexes live sockets to the match engine and privacy pipeline.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*client

	handler      func(msg model.WSClientMsg, c *client)
	pingInterval time.Duration
	serverTime   time.Time
}

func NewHub(pingInterval time.Duration) *Hub {
	return &Hub{
		clients:      map[string]*client{},
		pingInterval: pingInterval,
		serverTime:   time.Now(),
	}
}

// SetHandler wires the per-message dispatch (set by Server after engine setup).
func (h *Hub) SetHandler(fn func(msg model.WSClientMsg, c *client)) { h.handler = fn }

func (h *Hub) Register(c *client) {
	h.mu.Lock()
	// evict any stale socket for the same identity
	if old, ok := h.clients[c.userID]; ok {
		close(old.send)
		delete(h.clients, c.userID)
	}
	h.clients[c.userID] = c
	h.mu.Unlock()
}

func (h *Hub) Unregister(c *client) {
	h.mu.Lock()
	if cur, ok := h.clients[c.userID]; ok && cur == c {
		delete(h.clients, c.userID)
	}
	h.mu.Unlock()
}

func (h *Hub) push(c *client, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
		slog.Warn("dropping WS message: send buffer full", "user", c.userID)
	}
}

// --- match.Notifier -----------------------------------------------------

func (h *Hub) NotifyMatch(userID string, m *model.Match, latencyMs int64) {
	msg := model.WSServerMsg{Type: "match", Match: m, LatencyMs: latencyMs}
	h.broadcastUser(userID, msg)
	// dashboard observers receive all match events
	h.broadcastDash(func(c *client) { h.push(c, msg) })
}

func (h *Hub) NotifyRadar(userID string, state *model.RadarState) {
	msg := model.WSServerMsg{Type: "radar", Radar: state}
	h.broadcastUser(userID, msg)
	h.broadcastDash(func(c *client) { h.push(c, msg) })
}

// Pipeline broadcasts scrubbing events to command-center observers.
func (h *Hub) Pipeline(ev privacy.Event) {
	scrub := &model.ScrubEvent{
		ID: ev.ID, Type: ev.Type, UserID: ev.UserID,
		Pipeline: ev.Processor, RawLength: ev.RawLen, RedactedLen: ev.RedactLen,
		Tokens: ev.Tokens, Blocked: ev.Blocked, At: ev.At,
	}
	if ev.Raw != "" {
		scrub.Raw = ev.Raw
	}
	h.broadcastDash(func(c *client) {
		h.push(c, model.WSServerMsg{Type: "pipeline", Scrub: scrub})
	})
}

// Sweep notifies user-scoped sockets with live TTL countdowns (courtesy of the
// engine's ephemeral TTL mirror). Only the mobile user IS the peer, so we send
// identity "match_ttl" with remaining seconds.
func (h *Hub) MatchTTL(userID string, matchID string, remainingS int64) {
	h.broadcastUser(userID, model.WSServerMsg{
		Type: "match_ttl", Message: matchID, TTLSeconds: remainingS,
	})
}

func (h *Hub) broadcastUser(userID string, msg model.WSServerMsg) {
	data, _ := json.Marshal(msg)
	h.mu.RLock()
	if c, ok := h.clients[userID]; ok && !c.dash {
		select {
		case c.send <- data:
		default:
			slog.Warn("dropping match push", "user", userID)
		}
	}
	h.mu.RUnlock()
}

func (h *Hub) broadcastDash(fn func(c *client)) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		if c.dash {
			fn(c)
		}
	}
}

// readPump consumes mobile client messages (presence / scan / ping).
func (h *Hub) readPump(c *client) {
	defer func() {
		h.Unregister(c)
		c.conn.Close()
	}()
	c.conn.SetReadLimit(wsReadLimit)
	c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Debug("ws read error", "user", c.userID, "err", err)
			}
			return
		}
		var msg model.WSClientMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			slog.Warn("bad ws message", "user", c.userID, "err", err)
			continue
		}
		if h.handler != nil {
			h.handler(msg, c)
		}
	}
}

// writePump flushes outbound frames plus protocol heartbeats.
func (h *Hub) writePump(c *client) {
	ticker := time.NewTicker(h.pingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// serveWS upgrades an HTTP request to a live connection.
func (h *Hub) serveWS(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if userID == "" {
		http.Error(w, "user_id query param required", http.StatusBadRequest)
		return
	}
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin:     func(*http.Request) bool { return true }, // LAN demo
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade failed", "err", err)
		return
	}
	c := &client{
		conn:   conn,
		send:   make(chan []byte, wsPushBuffer),
		userID: userID,
		dash:   dashboardIDs[userID],
	}
	h.Register(c)
	if c.dash {
		h.push(c, model.WSServerMsg{Type: "hello", Message: "command-center observer linked"})
	} else {
		h.push(c, model.WSServerMsg{Type: "hello", Message: "presence stream ready"})
	}
	go h.writePump(c)
	go h.readPump(c)
}