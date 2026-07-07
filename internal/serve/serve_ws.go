package serve

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // same-origin is enforced by auth middleware
	},
}

// wsEvents handles WebSocket upgrade requests. It mirrors the SSE /events
// endpoint but over a bidirectional WebSocket: the server pushes typed event
// frames, and the client can send JSON-RPC-style commands (submit, cancel,
// approve) on the same connection.
func (s *Server) wsEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("serve: websocket upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	// Subscribe to the event broadcaster.
	ch, unsubscribe := s.bc.Subscribe()
	defer unsubscribe()

	// Write deadline pinger — keep the connection alive.
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start a ping ticker.
	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	// Read commands from the client in a separate goroutine.
	cmdCh := make(chan wsCommand, 8)
	errCh := make(chan error, 1)
	go func() {
		defer close(cmdCh)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			var cmd wsCommand
			if err := json.Unmarshal(message, &cmd); err != nil {
				continue // ignore malformed messages
			}
			cmdCh <- cmd
		}
	}()

	// Main event loop.
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "stream closed"))
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(data)); err != nil {
				return
			}
		case cmd, ok := <-cmdCh:
			if !ok {
				return
			}
			s.handleWSCommand(conn, r, cmd)
		case <-pingTicker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case err := <-errCh:
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("serve: websocket closed unexpectedly", "err", err)
			}
			return
		case <-r.Context().Done():
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server shutting down"))
			return
		}
	}
}

// wsCommand is a simple JSON-RPC-style command from the WebSocket client.
type wsCommand struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// handleWSCommand dispatches a WebSocket command to the appropriate handler.
func (s *Server) handleWSCommand(conn *websocket.Conn, r *http.Request, cmd wsCommand) {
	switch cmd.Method {
	case "submit":
		var params struct {
			Input string `json:"input"`
		}
		if err := json.Unmarshal(cmd.Params, &params); err != nil || params.Input == "" {
			s.writeWSError(conn, "invalid submit params")
			return
		}
		s.ctl().Submit(params.Input)
	case "cancel":
		s.ctl().Cancel()
	case "approve":
		var params struct {
			ID     string `json:"id"`
			Approve bool   `json:"approve"`
		}
		if err := json.Unmarshal(cmd.Params, &params); err != nil || params.ID == "" {
			s.writeWSError(conn, "invalid approve params")
			return
		}
		s.ctl().Approve(params.ID, params.Approve, true, true)
	default:
		s.writeWSError(conn, fmt.Sprintf("unknown method: %s", cmd.Method))
	}
}

func (s *Server) writeWSError(conn *websocket.Conn, msg string) {
	data, _ := json.Marshal(map[string]string{"error": msg})
	conn.WriteMessage(websocket.TextMessage, data)
}
