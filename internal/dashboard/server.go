// Package dashboard serves the logitcat web UI and SSE/API endpoints.
package dashboard

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/sammug/logitcat/internal/control"
)

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

//go:embed static
var staticFiles embed.FS

// Server exposes the dashboard HTTP server.
type Server struct {
	addr   string
	stats  *control.Stats
	broker *control.Broker
}

// NewServer creates a dashboard Server bound to addr (e.g. ":9090").
func NewServer(addr string, stats *control.Stats, broker *control.Broker) *Server {
	return &Server{addr: addr, stats: stats, broker: broker}
}

// Listen registers routes and starts the HTTP server. Blocks until error.
func (s *Server) Listen() error {
	mux := http.NewServeMux()

	// Static files (embedded)
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	// API endpoints
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/events", s.handleSSE)
	mux.HandleFunc("/api/alert", s.handlePushAlert)

	log.Printf("dashboard: listening on http://localhost%s", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// handlePushAlert accepts a TailEvent JSON body from pipe mode and
// broadcasts it to all SSE subscribers — allowing pipe + daemon to coexist.
func (s *Server) handlePushAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var event control.TailEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return
	}
	s.broker.Publish(event)
	s.stats.IncAlerts()
	w.WriteHeader(http.StatusNoContent)
}

// ForwardAlert sends a TailEvent to a running daemon's /api/alert endpoint.
// Returns true if the daemon accepted it.
func ForwardAlert(port string, event control.TailEvent) bool {
	data, err := json.Marshal(event)
	if err != nil {
		return false
	}
	resp, err := http.Post(
		fmt.Sprintf("http://localhost%s/api/alert", port),
		"application/json",
		bytesReader(data),
	)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusNoContent
}

// IsDaemonRunning returns true if a logitcat dashboard is reachable on port.
func IsDaemonRunning(port string) bool {
	resp, err := http.Get(fmt.Sprintf("http://localhost%s/api/status", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	resp := map[string]any{
		"pid":         s.stats.PID(),
		"uptime":      time.Since(s.stats.StartTime).Round(time.Second).String(),
		"config_path": s.stats.ConfigPath,
		"files":       s.stats.Files,
		"rule_count":  s.stats.RuleCount,
		"alert_count": s.stats.AlertCount(),
	}
	json.NewEncoder(w).Encode(resp)
}

// handleSSE streams TailEvents as Server-Sent Events.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ch := s.broker.Subscribe()
	defer s.broker.Unsubscribe(ch)

	// Keep-alive ping every 15s so the browser doesn't drop the connection.
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: alert\ndata: %s\n\n", data)
			flusher.Flush()

		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}
