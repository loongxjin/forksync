package app

import (
	"context"
	"net/http"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/logger"

	"github.com/gorilla/websocket"
)

// upgrader permits the local Electron origin only. The server listens on
// 127.0.0.1 so this is process-local traffic; we allow all origins because
// the WS handshake from Electron carries no Origin header we can rely on.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// registerStreamRoutes wires the WebSocket endpoints.
func (s *Server) registerStreamRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/stream/resolve/{name}", s.handleResolveStream)
	mux.HandleFunc("/stream/events", s.handleEventsStream)
}

// handleResolveStream upgrades to WebSocket and streams agent resolve events.
//
// Each WS text message is one JSON-encoded agent.StreamEvent (same shape as
// the old NDJSON lines, with the `t` discriminator the frontend expects). The
// terminal `done` / `error` event is sent last, then the server closes the
// connection. If the client disconnects (closes the WS), the request context
// is cancelled which triggers the rollback guard inside runResolveWithAgent —
// replacing the old SIGINT/SIGTERM signal guard in cmd/resolve.go.
func (s *Server) handleResolveStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "missing repo name", http.StatusBadRequest)
		return
	}

	r2, ok := s.deps.Store.GetByName(name)
	if !ok {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}

	// Query params mirror the old CLI flags: ?agent=&noConfirm=
	req := resolveRequest{
		Mode:      "agent",
		Agent:     r.URL.Query().Get("agent"),
		NoConfirm: r.URL.Query().Get("noConfirm") == "true",
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Warn("app: ws upgrade failed", "repo", name, "error", err)
		return
	}
	defer conn.Close()

	// Derive a context that is cancelled when the WS peer closes (ReadMessage
	// returns an error) OR when the underlying request/server shuts down.
	// This cancellation drives the rollback guard inside runResolveWithAgent,
	// replacing the old SIGINT/SIGTERM signal guard in cmd/resolve.go.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	// sink pushes each StreamEvent to the client as a WS text message.
	sink := func(ev agent.StreamEvent) {
		if werr := conn.WriteJSON(ev); werr != nil {
			logger.Debug("app: ws write failed", "repo", name, "type", ev.Type, "error", werr)
		}
	}

	// Run the resolve flow; terminal done/error is emitted by the flow itself.
	_, runErr := s.runResolveWithAgent(ctx, r2, req, sink)
	if runErr != nil {
		_ = conn.WriteJSON(agent.StreamEvent{
			Type: agent.StreamEventError,
			Data: runErr.Error(),
		})
	}
}

// eventsMessage is the envelope pushed to /stream/events subscribers. Each
// message tells the client which slice of state changed so it can re-fetch
// just that slice (repos or history).
type eventsMessage struct {
	Type string `json:"type"` // "repos_changed" | "history_changed" | "ready"
}

// handleEventsStream upgrades to WebSocket and pushes a message every time the
// engine's state changes (repo add/update/remove, history insert/summary/clear).
// This replaces the renderer's fixed-interval status/history polling: the
// client opens one WS on launch and reacts to push events instead of polling.
//
// On connect it sends {"type":"ready"} so the client knows the channel is live
// (and can do an initial full fetch). The auth token is read from ?token=
// by the authToken middleware before this handler runs.
func (s *Server) handleEventsStream(w http.ResponseWriter, r *http.Request) {
	if s.deps.Bus == nil {
		http.Error(w, "event bus unavailable", http.StatusServiceUnavailable)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Warn("app: events ws upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	// Signal readiness so the client does its initial fetch.
	if werr := conn.WriteJSON(eventsMessage{Type: "ready"}); werr != nil {
		logger.Debug("app: events ws initial write failed", "error", werr)
		return
	}

	ch, cancel := s.deps.Bus.Subscribe()
	defer cancel()

	// Pump: forward bus events to the WS. A nil channel means the bus was
	// closed (server shutdown) — end the loop.
	ctx, cancelCtx := context.WithCancel(r.Context())
	defer cancelCtx()
	go func() {
		// Read loop: a read error (client disconnect) cancels the context so the
		// select below unblocks and we unsubscribe.
		defer cancelCtx()
		for {
			if _, _, rerr := conn.ReadMessage(); rerr != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				// Bus closed (server shutdown). Best-effort close frame.
				return
			}
			if werr := conn.WriteJSON(eventsMessage{Type: string(ev.Type)}); werr != nil {
				logger.Debug("app: events ws write failed", "type", ev.Type, "error", werr)
				return
			}
		}
	}
}
