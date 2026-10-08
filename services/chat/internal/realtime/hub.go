package realtime

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"github.com/kanarinn/Communication-app/services/chat/internal/store"
)

const (
	channel      = "chat:messages"
	maxBodyLen   = 4000
	writeTimeout = 10 * time.Second
	pingInterval = 30 * time.Second
	pongTimeout  = 60 * time.Second
)

type inbound struct {
	Type string `json:"type"` // "send"
	To   string `json:"to"`
	Body string `json:"body"`
}

type outbound struct {
	Type    string         `json:"type"` // "message" | "error"
	Message *store.Message `json:"message,omitempty"`
	Error   string         `json:"error,omitempty"`
}

type client struct {
	userID string
	conn   *websocket.Conn
	send   chan []byte
}

type Hub struct {
	rdb   *redis.Client
	store *store.Store

	mu      sync.RWMutex
	clients map[string]map[*client]struct{} // userID -> open connections
}

func NewHub(rdb *redis.Client, st *store.Store) *Hub {
	return &Hub{rdb: rdb, store: st, clients: map[string]map[*client]struct{}{}}
}

func (h *Hub) Run(ctx context.Context) error {
	sub := h.rdb.Subscribe(ctx, channel)
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil {
		return err
	}
	for msg := range sub.Channel() {
		var m store.Message
		if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
			log.Printf("hub: bad payload: %v", err)
			continue
		}
		payload, _ := json.Marshal(outbound{Type: "message", Message: &m})
		h.deliver(m.SenderID, payload)
		if m.RecipientID != m.SenderID {
			h.deliver(m.RecipientID, payload)
		}
	}
	return ctx.Err()
}

func (h *Hub) deliver(userID string, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients[userID] {
		select {
		case c.send <- payload:
		default:
			// Slow consumer: drop rather than block the whole hub.
		}
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error
	}
	c := &client{userID: userID, conn: conn, send: make(chan []byte, 64)}
	h.add(c)
	go c.writeLoop()
	h.readLoop(r.Context(), c)
	h.remove(c)
}

func (h *Hub) add(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.userID] == nil {
		h.clients[c.userID] = map[*client]struct{}{}
	}
	h.clients[c.userID][c] = struct{}{}
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[c.userID], c)
	if len(h.clients[c.userID]) == 0 {
		delete(h.clients, c.userID)
	}
	// Safe to close here: deliver() only sends while holding the read lock.
	close(c.send)
}

func (h *Hub) readLoop(ctx context.Context, c *client) {
	c.conn.SetReadLimit(16 << 10)
	c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var in inbound
		if err := json.Unmarshal(data, &in); err != nil {
			c.fail("invalid json")
			continue
		}
		switch in.Type {
		case "send":
			if msg := h.handleSend(ctx, c, in); msg != "" {
				c.fail(msg)
			}
		default:
			c.fail("unknown message type")
		}
	}
}

func (h *Hub) handleSend(ctx context.Context, c *client, in inbound) string {
	body := strings.TrimSpace(in.Body)
	switch {
	case in.To == "":
		return "missing recipient"
	case body == "":
		return "empty message"
	case len(body) > maxBodyLen:
		return "message too long"
	}
	friends, err := h.store.AreFriends(ctx, c.userID, in.To)
	if err != nil {
		log.Printf("hub: AreFriends: %v", err)
		return "database error"
	}
	if !friends {
		return "you can only message friends"
	}
	m, err := h.store.SaveMessage(ctx, c.userID, in.To, body)
	if err != nil {
		log.Printf("hub: SaveMessage: %v", err)
		return "database error"
	}
	payload, _ := json.Marshal(m)
	if err := h.rdb.Publish(ctx, channel, payload).Err(); err != nil {
		log.Printf("hub: publish: %v", err)
		return "delivery failed"
	}
	return ""
}

func (c *client) fail(msg string) {
	payload, _ := json.Marshal(outbound{Type: "error", Error: msg})
	select {
	case c.send <- payload:
	default:
	}
}

func (c *client) writeLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	defer c.conn.Close()
	for {
		select {
		case payload, ok := <-c.send:
			if !ok {
				c.conn.WriteControl(websocket.CloseMessage, nil, time.Now().Add(writeTimeout))
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeTimeout)); err != nil {
				return
			}
		}
	}
}
