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

// safeConn wraps a websocket.Conn with a per-connection write mutex
// to prevent concurrent writes which cause gorilla/websocket to panic.
type safeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (sc *safeConn) writeMessage(messageType int, data []byte) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return sc.conn.WriteMessage(messageType, data)
}

func (sc *safeConn) close() {
	sc.conn.Close()
}

type wsHub struct {
	mu            sync.RWMutex
	clients       map[string]map[*safeConn]bool
	globalClients map[*safeConn]bool // for notification push
}

func newWSHub() *wsHub {
	return &wsHub{
		clients:       make(map[string]map[*safeConn]bool),
		globalClients: make(map[*safeConn]bool),
	}
}

func (h *wsHub) addGlobal(sc *safeConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.globalClients[sc] = true
}

func (h *wsHub) removeGlobal(sc *safeConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.globalClients, sc)
}

func (h *wsHub) broadcastNotification(event []byte) {
	h.mu.RLock()
	conns := make([]*safeConn, 0, len(h.globalClients))
	for c := range h.globalClients {
		conns = append(conns, c)
	}
	h.mu.RUnlock()
	for _, sc := range conns {
		if err := sc.writeMessage(websocket.TextMessage, event); err != nil {
			slog.Debug("notification ws write error", "error", err)
			sc.close()
			h.removeGlobal(sc)
		}
	}
}

func (h *wsHub) add(sessionID string, sc *safeConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[sessionID] == nil {
		h.clients[sessionID] = make(map[*safeConn]bool)
	}
	h.clients[sessionID][sc] = true
}

func (h *wsHub) remove(sessionID string, sc *safeConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.clients[sessionID]; ok {
		delete(conns, sc)
		if len(conns) == 0 {
			delete(h.clients, sessionID)
		}
	}
}

func (h *wsHub) broadcast(sessionID string, data []byte) {
	h.mu.RLock()
	conns := make([]*safeConn, 0, len(h.clients[sessionID]))
	for sc := range h.clients[sessionID] {
		conns = append(conns, sc)
	}
	h.mu.RUnlock()
	for _, sc := range conns {
		if err := sc.writeMessage(websocket.BinaryMessage, data); err != nil {
			slog.Debug("ws write error", "error", err)
			sc.close()
			h.remove(sessionID, sc)
		}
	}
}

func (s *Server) handleNotificationWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("notification ws upgrade failed", "error", err)
		return
	}
	defer conn.Close()
	sc := &safeConn{conn: conn}
	s.hub.addGlobal(sc)
	defer s.hub.removeGlobal(sc)
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
	defer conn.Close()
	sc := &safeConn{conn: conn}

	// If session is still provisioning (e.g. Docker sandbox), wait for it
	if sess.GetState() == session.StateStarting {
		sc.writeMessage(websocket.BinaryMessage, []byte("\x1b[36mProvisioning Docker sandbox...\x1b[0m\r\n"))
		for i := 0; i < 120; i++ { // wait up to 2 minutes
			time.Sleep(1 * time.Second)
			state := sess.GetState()
			if state == session.StateRunning {
				sc.writeMessage(websocket.BinaryMessage, []byte("\x1b[32mSandbox ready!\x1b[0m\r\n"))
				break
			}
			if state == session.StateErrored {
				errMsg := sess.GetError()
				sc.writeMessage(websocket.BinaryMessage, []byte("\x1b[31mSandbox failed: "+errMsg+"\x1b[0m\r\n"))
				return
			}
		}
		if sess.GetState() == session.StateStarting {
			sc.writeMessage(websocket.BinaryMessage, []byte("\x1b[31mSandbox provisioning timed out\x1b[0m\r\n"))
			return
		}
	}

	s.hub.add(sessionID, sc)
	defer s.hub.remove(sessionID, sc)
	// Replay ring buffer
	if buf := sess.Output().Bytes(); len(buf) > 0 {
		sc.writeMessage(websocket.BinaryMessage, buf)
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
				sess.Resize(uint16(msg.Rows), uint16(msg.Cols))
				continue
			}
			mgr.WriteInput(sessionID, data)
		case websocket.BinaryMessage:
			mgr.WriteInput(sessionID, data)
		}
	}
}
