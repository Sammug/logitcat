package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sammug/logwatch/config"
	"github.com/sammug/logwatch/internal/control"
	"github.com/sammug/logwatch/internal/daemon"
	"github.com/sammug/logwatch/internal/dashboard"
	"github.com/sammug/logwatch/internal/dispatcher"
	"github.com/sammug/logwatch/internal/parser"
	"github.com/sammug/logwatch/internal/rules"
	"github.com/sammug/logwatch/internal/watcher"
)

const usage = `logwatch — lightweight log parser and alerting engine

Usage:
  logwatch start  <config.ini>   start daemon in the background
  logwatch stop                  stop the running daemon
  logwatch status                show daemon health and metrics
  logwatch tail                  stream live alerts from the daemon
  logwatch reload                hot-reload the daemon's config
  logwatch run    <config.ini>   run in the foreground (dev mode)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "start":
		cmdStart()
	case "stop":
		cmdStop()
	case "status":
		cmdStatus()
	case "tail":
		cmdTail()
	case "reload":
		cmdReload()
	case "run":
		cmdRun(false)
	case "--daemon": // internal flag used by start to run the actual daemon
		cmdRun(true)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n%s", os.Args[1], usage)
		os.Exit(1)
	}
}

// ── subcommands ───────────────────────────────────────────────────────────────

func cmdStart() {
	if len(os.Args) < 3 {
		fatalf("usage: logwatch start <config.ini>")
	}
	if running, pid := daemon.IsRunning(); running {
		fatalf("logwatch is already running (PID %d)", pid)
	}
	// Re-launch self with --daemon flag so the child runs the actual pipeline.
	if err := daemon.Start([]string{"--daemon", os.Args[2]}); err != nil {
		fatalf("%v", err)
	}
}

func cmdStop() {
	client := control.NewClient(daemon.SocketPath())
	resp, err := client.Stop()
	if err != nil {
		fatalf("logwatch is not running or unreachable: %v", err)
	}
	fmt.Println("logwatch stopped:", resp.Message)
}

func cmdStatus() {
	client := control.NewClient(daemon.SocketPath())
	s, err := client.Status()
	if err != nil {
		fmt.Println("● logwatch is not running")
		os.Exit(1)
	}
	fmt.Printf("● logwatch is running (PID %d)\n", s.PID)
	fmt.Printf("  Uptime:    %s\n", s.Uptime)
	fmt.Printf("  Config:    %s\n", s.ConfigPath)
	fmt.Printf("  Watching:  %d file(s)\n", len(s.Files))
	for _, f := range s.Files {
		fmt.Printf("             %s\n", f)
	}
	fmt.Printf("  Rules:     %d active\n", s.RuleCount)
	fmt.Printf("  Alerts:    %d fired\n", s.AlertCount)
}

func cmdTail() {
	client := control.NewClient(daemon.SocketPath())
	fmt.Println("Streaming alerts (Ctrl+C to stop)...")
	err := client.Tail(func(e control.TailEvent) bool {
		fmt.Printf("[%s] [%-8s] [%s] %s\n", e.Time, e.Severity, e.Rule, e.Message)
		return true
	})
	if err != nil {
		fatalf("tail: %v", err)
	}
}

func cmdReload() {
	client := control.NewClient(daemon.SocketPath())
	resp, err := client.Reload()
	if err != nil {
		fatalf("reload failed: %v", err)
	}
	fmt.Println("logwatch:", resp.Message)
}

// cmdRun starts the full pipeline — either in foreground (dev) or as the daemon process.
func cmdRun(isDaemon bool) {
	if len(os.Args) < 3 {
		fatalf("usage: logwatch run <config.ini>")
	}
	configPath := os.Args[2]

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Runtime shared state
	stats := &control.Stats{
		StartTime:  time.Now(),
		ConfigPath: configPath,
		Files:      cfg.Files,
		RuleCount:  len(cfg.Rules),
	}
	broker := control.NewBroker()

	if isDaemon {
		if err := daemon.WritePID(); err != nil {
			log.Fatalf("daemon: cannot write PID: %v", err)
		}
		defer daemon.RemovePID()
	}

	// Pipeline channels
	rawLines := make(chan parser.RawLine, 1000)
	entries  := make(chan parser.LogEntry, 1000)
	alerts   := make(chan rules.Alert, 100)

	// Control server (IPC)
	quit := make(chan struct{})
	srv := control.NewServer(
		daemon.SocketPath(),
		stats,
		broker,
		func() { close(quit) }, // stop callback
		func() { /* reload: TODO */ },
	)
	go func() {
		if err := srv.Listen(); err != nil {
			log.Printf("control server: %v", err)
		}
	}()

	// Pipeline stages
	go watcher.Watch(cfg.Files, rawLines)
	go parser.Run(rawLines, entries)
	go rules.Match(cfg.Rules, entries, alerts)
	go dispatcher.Dispatch(cfg, alerts, stats, broker)

	// Dashboard (optional)
	if cfg.DashboardAddr != "" {
		dash := dashboard.NewServer(cfg.DashboardAddr, stats, broker)
		go func() {
			if err := dash.Listen(); err != nil {
				log.Printf("dashboard: %v", err)
			}
		}()
	}

	log.Printf("logwatch started — watching %d file(s), %d rule(s) active",
		len(cfg.Files), len(cfg.Rules))
	if cfg.DashboardAddr != "" {
		log.Printf("dashboard: http://localhost%s", cfg.DashboardAddr)
	}

	// Shutdown on SIGINT / SIGTERM or stop command
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sig:
	case <-quit:
	}

	log.Println("logwatch stopped.")
	srv.Close()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
