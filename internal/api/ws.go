package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// wsConn couples a websocket connection with the write lock gorilla/websocket
// requires: concurrent WriteMessage calls on one connection panic with
// "concurrent write to websocket connection" (which would kill the whole
// panel process from the collector goroutine) or silently corrupt frames.
type wsConn struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

// write sends one frame; the deadline and the write are serialized under the
// same lock so Broadcast and the pinger can never interleave.
func (c *wsConn) write(msgType int, data []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.conn.WriteMessage(msgType, data)
}

func (c *wsConn) close() {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.conn.Close()
}

// Hub fans live traffic updates out to the admin UI over WebSocket.
type Hub struct {
	mu    sync.Mutex
	conns map[*wsConn]struct{}
	allow func(r *http.Request) bool
}

// NewHub builds the live-update hub. corsOrigin mirrors the API CORS policy:
// "" / "*" keep the socket same-origin-only; an explicit origin is also
// accepted cross-site.
func NewHub(corsOrigin string) *Hub {
	return &Hub{
		conns: map[*wsConn]struct{}{},
		allow: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // non-browser clients (curl, custom tooling)
			}
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			if u.Host != "" && u.Host == r.Host {
				return true // same-origin (the embedded panel UI)
			}
			return corsOrigin != "" && corsOrigin != "*" && origin == corsOrigin
		},
	}
}

func (h *Hub) add(c *wsConn)    { h.mu.Lock(); h.conns[c] = struct{}{}; h.mu.Unlock() }
func (h *Hub) remove(c *wsConn) { h.mu.Lock(); delete(h.conns, c); h.mu.Unlock() }

// Broadcast marshals v once and writes it to every connected admin.
func (h *Hub) Broadcast(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	conns := make([]*wsConn, 0, len(h.conns))
	for c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.Unlock()
	for _, c := range conns {
		if err := c.write(websocket.TextMessage, data); err != nil {
			c.close()
			h.remove(c)
		}
	}
}

// Serve upgrades the request and keeps the connection until the peer closes.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		// Cross-site WS hijack guard: the session cookie is the auth, so the
		// Origin header must prove the handshake comes from this panel (or an
		// explicitly allowed origin) — not from a random website.
		CheckOrigin: h.allow,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &wsConn{conn: conn}
	h.add(c)
	defer func() {
		h.remove(c)
		c.close()
	}()
	conn.SetReadLimit(4096)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
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
				if err := c.write(websocket.PingMessage, nil); err != nil {
					c.close()
					return
				}
			}
		}
	}()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
