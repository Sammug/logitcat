package control

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)


// Stats holds live daemon metrics updated atomically by the pipeline.
type Stats struct {
	StartTime  time.Time
	ConfigPath string
	Files      []string
	RuleCount  int
	alerts     atomic.Int64
}

func (s *Stats) IncAlerts()        { s.alerts.Add(1) }
func (s *Stats) AlertCount() int64 { return s.alerts.Load() }
func (s *Stats) PID() int          { return os.Getpid() }

// Server listens on the UNIX socket and handles CLI commands.
type Server struct {
	socketPath string
	stats      *Stats
	broker     *Broker
	stopFn     func() // called when the daemon receives a stop command
	reloadFn   func() // called on reload
	listener   net.Listener
}

// NewServer creates a Server. stopFn and reloadFn are callbacks into the daemon.
func NewServer(socketPath string, stats *Stats, broker *Broker, stopFn, reloadFn func()) *Server {
	return &Server{
		socketPath: socketPath,
		stats:      stats,
		broker:     broker,
		stopFn:     stopFn,
		reloadFn:   reloadFn,
	}
}

// Listen starts accepting connections. It blocks until the socket is closed.
func (s *Server) Listen() error {
	_ = os.Remove(s.socketPath) // clean up stale socket

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("control socket: %w", err)
	}
	s.listener = ln
	defer ln.Close()

	log.Printf("control: listening on %s", s.socketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			return nil // socket closed — normal shutdown
		}
		go s.handle(conn)
	}
}

// Close shuts down the listener.
func (s *Server) Close() {
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	var cmd Command
	if err := json.NewDecoder(conn).Decode(&cmd); err != nil {
		return
	}

	switch cmd.Type {
	case CmdStatus:
		s.handleStatus(conn)
	case CmdReload:
		s.handleReload(conn)
	case CmdStop:
		s.handleStop(conn)
	case CmdTail:
		s.handleTail(conn)
	}
}

func (s *Server) handleStatus(conn net.Conn) {
	uptime := time.Since(s.stats.StartTime).Round(time.Second).String()
	resp := StatusResponse{
		PID:        os.Getpid(),
		Uptime:     uptime,
		ConfigPath: s.stats.ConfigPath,
		Files:      s.stats.Files,
		RuleCount:  s.stats.RuleCount,
		AlertCount: s.stats.AlertCount(),
	}
	_ = json.NewEncoder(conn).Encode(resp)
}

func (s *Server) handleReload(conn net.Conn) {
	s.reloadFn()
	_ = json.NewEncoder(conn).Encode(AckResponse{OK: true, Message: "config reloaded"})
}

func (s *Server) handleStop(conn net.Conn) {
	_ = json.NewEncoder(conn).Encode(AckResponse{OK: true, Message: "stopping"})
	conn.Close()
	s.stopFn()
}

// handleTail keeps the connection open and streams TailEvents as they arrive.
func (s *Server) handleTail(conn net.Conn) {
	ch := s.broker.Subscribe()
	defer s.broker.Unsubscribe(ch)

	enc := json.NewEncoder(conn)
	for event := range ch {
		if err := enc.Encode(event); err != nil {
			return // client disconnected
		}
	}
}

// ── Broker ────────────────────────────────────────────────────────────────────

// Broker is a simple pub/sub hub for streaming TailEvents to connected clients.
type Broker struct {
	mu   sync.RWMutex
	subs map[chan TailEvent]struct{}
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[chan TailEvent]struct{})}
}

func (b *Broker) Subscribe() chan TailEvent {
	ch := make(chan TailEvent, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Broker) Unsubscribe(ch chan TailEvent) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
	close(ch)
}

// Close unsubscribes all subscribers — used by pipe mode on EOF.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}

// Publish broadcasts an event to all active tail subscribers.
// Slow subscribers are skipped (non-blocking send).
func (b *Broker) Publish(e TailEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // drop rather than block the pipeline
		}
	}
}
