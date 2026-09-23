package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Hub fans live traffic updates out to the admin UI over WebSocket.
type Hub struct {
	mu       sync.Mutex
	conns    map[*websocket.Conn]struct{}
	upgrader websocket.Upgrader
}

func NewHub() *Hub {
	return &Hub{
		conns: map[*websocket.Conn]struct{}{},
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 4096,
			// the session cookie is the real auth; allow the CORS-configured
			// origins through (the browser sends credentials cross-origin)
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *Hub) add(c *websocket.Conn)    { h.mu.Lock(); h.conns[c] = struct{}{}; h.mu.Unlock() }
func (h *Hub) remove(c *websocket.Conn) { h.mu.Lock(); delete(h.conns, c); h.mu.Unlock() }

// Broadcast marshals v once and writes it to every connected admin.
func (h *Hub) Broadcast(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(h.conns))
	for c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.Unlock()
	for _, c := range conns {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
			_ = c.Close()
			h.remove(c)
		}
	}
}

// Serve upgrades the request and keeps the connection until the peer closes.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request) {
	c, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.add(c)
	defer func() {
		h.remove(c)
		_ = c.Close()
	}()
	c.SetReadLimit(4096)
	_ = c.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.SetPongHandler(func(string) error {
		return c.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	// pinger keeps proxies from idling the tunnel out
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
					_ = c.Close()
					return
				}
			}
		}
	}()
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
}
