package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/IgorDeo/claude-websessions/internal/session"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsMessage struct {
	Type string `json:"type"`
	Rows int    `json:"rows,omitempty"`
	Cols int    `json:"cols,omitempty"`
}

type wsHub struct {
	mu            sync.RWMutex
	clients       map[string]map[*websocket.Conn]bool
	globalClients map[*websocket.Conn]bool // for notification push
}

func newWSHub() *wsHub {
	return &wsHub{
		clients:       make(map[string]map[*websocket.Conn]bool),
		globalClients: make(map[*websocket.Conn]bool),
	}
}

func (h *wsHub) addGlobal(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.globalClients[conn] = true
}

func (h *wsHub) removeGlobal(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.globalClients, conn)
}

func (h *wsHub) broadcastNotification(event []byte) {
	h.mu.RLock()
	conns := make(map[*websocket.Conn]bool)
	for c := range h.globalClients {
		conns[c] = true
	}
	h.mu.RUnlock()
	for conn := range conns {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, event); err != nil {
			slog.Debug("notification ws write error", "error", err)
			_ = conn.Close()
			h.removeGlobal(conn)
		}
	}
}

func (h *wsHub) add(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[sessionID] == nil {
		h.clients[sessionID] = make(map[*websocket.Conn]bool)
	}
	h.clients[sessionID][conn] = true
}

func (h *wsHub) remove(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.clients[sessionID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.clients, sessionID)
		}
	}
}

func (h *wsHub) broadcast(sessionID string, data []byte) {
	h.mu.RLock()
	conns := h.clients[sessionID]
	h.mu.RUnlock()
	for conn := range conns {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			slog.Debug("ws write error", "error", err)
			_ = conn.Close()
			h.remove(sessionID, conn)
		}
	}
}

func (s *Server) handleNotificationWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("notification ws upgrade failed", "error", err)
		return
	}
	defer conn.Close() //nolint:errcheck
	s.hub.addGlobal(conn)
	defer s.hub.removeGlobal(conn)
	// Keep connection alive, read pings
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request, sessionID string, mgr *session.Manager) {
	sess, ok := mgr.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade failed", "error", err)
		return
	}
	defer conn.Close() //nolint:errcheck

	// If session is still provisioning (e.g. Docker sandbox), wait for it
	if sess.GetState() == session.StateStarting {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("\x1b[36mProvisioning Docker sandbox...\x1b[0m\r\n"))
		for i := 0; i < 120; i++ { // wait up to 2 minutes
			time.Sleep(1 * time.Second)
			state := sess.GetState()
			if state == session.StateRunning {
				_ = conn.WriteMessage(websocket.BinaryMessage, []byte("\x1b[32mSandbox ready!\x1b[0m\r\n"))
				break
			}
			if state == session.StateErrored {
				errMsg := sess.GetError()
				_ = conn.WriteMessage(websocket.BinaryMessage, []byte("\x1b[31mSandbox failed: "+errMsg+"\x1b[0m\r\n"))
				return
			}
		}
		if sess.GetState() == session.StateStarting {
			_ = conn.WriteMessage(websocket.BinaryMessage, []byte("\x1b[31mSandbox provisioning timed out\x1b[0m\r\n"))
			return
		}
	}

	s.hub.add(sessionID, conn)
	defer s.hub.remove(sessionID, conn)
	// Replay ring buffer
	if buf := sess.Output().Bytes(); len(buf) > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_ = conn.WriteMessage(websocket.BinaryMessage, buf)
	}
	// Read user input
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		switch msgType {
		case websocket.TextMessage:
			var msg wsMessage
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type == "resize" {
				_ = sess.Resize(uint16(msg.Rows), uint16(msg.Cols))
				continue
			}
			_ = mgr.WriteInput(sessionID, data)
		case websocket.BinaryMessage:
			_ = mgr.WriteInput(sessionID, data)
		}
	}
}
