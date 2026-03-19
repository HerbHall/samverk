package server

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"
)

const (
	wsSendBufSize  = 64            // messages buffered per client before dropping
	wsPingInterval = 30 * time.Second
	wsPongTimeout  = 10 * time.Second
)

// Hub manages WebSocket client connections and broadcasts events.
type Hub struct {
	broadcast  chan []byte
	register   chan *wsClient
	unregister chan *wsClient
	clients    map[*wsClient]struct{}
	logger     *zap.Logger
}

type wsClient struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// NewHub creates a Hub ready to Run.
func NewHub(logger *zap.Logger) *Hub {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Hub{
		broadcast:  make(chan []byte, 256),
		register:   make(chan *wsClient, 16),
		unregister: make(chan *wsClient, 16),
		clients:    make(map[*wsClient]struct{}),
		logger:     logger,
	}
}

// Broadcast enqueues data for delivery to all connected clients.
// It never blocks; if the broadcast channel is full the message is dropped.
func (h *Hub) Broadcast(data []byte) {
	select {
	case h.broadcast <- data:
	default:
		h.logger.Warn("ws hub: broadcast channel full, message dropped")
	}
}

// Run starts the hub event loop. It blocks until ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Close all remaining clients.
			for c := range h.clients {
				h.closeClient(c)
			}
			return

		case c := <-h.register:
			h.clients[c] = struct{}{}
			h.logger.Debug("ws hub: client registered", zap.Int("total", len(h.clients)))

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
				h.logger.Debug("ws hub: client unregistered", zap.Int("total", len(h.clients)))
			}

		case msg := <-h.broadcast:
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// Slow client: buffer full — drop and disconnect.
					h.logger.Warn("ws hub: slow client, dropping connection")
					delete(h.clients, c)
					close(c.send)
				}
			}
		}
	}
}

// closeClient closes the underlying WebSocket connection and removes the
// client from the hub without sending on the unregister channel (called only
// from within the Run goroutine).
func (h *Hub) closeClient(c *wsClient) {
	delete(h.clients, c)
	close(c.send)
	_ = c.conn.Close(websocket.StatusNormalClosure, "server shutdown")
}

// writePump forwards messages from the send channel to the WebSocket
// connection. It also sends periodic pings and closes the connection if
// a pong is not received within wsPongTimeout.
func (c *wsClient) writePump(ctx context.Context) {
	ticker := time.NewTicker(wsPingInterval)
	defer func() {
		ticker.Stop()
		c.hub.unregister <- c
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case msg, ok := <-c.send:
			if !ok {
				// Hub closed the channel.
				_ = c.conn.Close(websocket.StatusNormalClosure, "")
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				c.hub.logger.Debug("ws write error", zap.Error(err))
				return
			}

		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, wsPongTimeout)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				c.hub.logger.Debug("ws ping timeout", zap.Error(err))
				return
			}
		}
	}
}

// handleWS upgrades an HTTP request to a WebSocket connection and registers
// the new client with the Hub. The Hub must be non-nil.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: false,
	})
	if err != nil {
		s.logger.Debug("ws upgrade failed", zap.Error(err))
		return
	}

	c := &wsClient{
		hub:  s.hub,
		conn: conn,
		send: make(chan []byte, wsSendBufSize),
	}
	s.hub.register <- c

	// writePump owns the connection lifetime. Use a background context so that
	// the pump is not cancelled when the HTTP request context ends (the HTTP
	// handler returns immediately after the upgrade).
	go c.writePump(context.Background())
}
