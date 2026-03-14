package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sammug/logitcat/config"
	"github.com/sammug/logitcat/internal/control"
	"github.com/sammug/logitcat/internal/daemon"
	"github.com/sammug/logitcat/internal/dashboard"
	"github.com/sammug/logitcat/internal/dispatcher"
	"github.com/sammug/logitcat/internal/parser"
	"github.com/sammug/logitcat/internal/rules"
	"github.com/sammug/logitcat/internal/watcher"
)

const usage = `logitcat — lightweight log parser and alerting engine

Usage:
  logitcat start  <config.ini>             start daemon in the background
  logitcat stop                            stop the running daemon
  logitcat status                          show daemon health and metrics
  logitcat tail                            stream live alerts from the daemon
  logitcat reload                          hot-reload the daemon's config
  logitcat run    <config.ini>             run in the foreground (dev mode)
  logitcat pipe   <config.ini> [--source <name>] [--dashboard]
                                           read from stdin, apply rules, alert

Examples:
  adb logcat | logitcat pipe android.ini --source adb-logcat --dashboard
  gradle build 2>&1 | logitcat pipe gradle.ini
  tail -f app.log   | logitcat pipe rules.ini --source myapp
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
	case "pipe":
		cmdPipe()
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
		fatalf("usage: logitcat start <config.ini>")
	}
	if running, pid := daemon.IsRunning(); running {
		fatalf("logitcat is already running (PID %d)", pid)
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
		fatalf("logitcat is not running or unreachable: %v", err)
	}
	fmt.Println("logitcat stopped:", resp.Message)
}

func cmdStatus() {
	client := control.NewClient(daemon.SocketPath())
	s, err := client.Status()
	if err != nil {
		fmt.Println("● logitcat is not running")
		os.Exit(1)
	}
	fmt.Printf("● logitcat is running (PID %d)\n", s.PID)
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
	fmt.Println("logitcat:", resp.Message)
}

// cmdPipe reads log lines from stdin, parses and matches them, then dispatches alerts.
// Flags: --source <name>  sets the source label (default "stdin")
//        --dashboard      also start the web dashboard
func cmdPipe() {
	if len(os.Args) < 3 {
		fatalf("usage: logitcat pipe <config.ini> [--source <name>] [--dashboard]")
	}
	configPath := os.Args[2]

	// Parse optional flags
	source := "stdin"
	showDash := false
	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--source":
			if i+1 < len(os.Args) {
				source = os.Args[i+1]
				i++
			}
		case "--dashboard":
			showDash = true
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	stats := &control.Stats{
		StartTime:  time.Now(),
		ConfigPath: configPath,
		Files:      []string{source},
		RuleCount:  len(cfg.Rules),
	}
	broker := control.NewBroker()

	// Subscribe to forwarding BEFORE pipeline starts so no events are missed.
	forwardDone := make(chan struct{})
	daemonRunning := showDash && dashboard.IsDaemonRunning(cfg.DashboardAddr)
	var forwardCh chan control.TailEvent
	if daemonRunning {
		forwardCh = broker.Subscribe()
		log.Printf("dashboard: daemon already running on %s — forwarding alerts to it", cfg.DashboardAddr)
		go func() {
			for event := range forwardCh {
				dashboard.ForwardAlert(cfg.DashboardAddr, event)
			}
			close(forwardDone)
		}()
	}

	// Pipeline channels
	rawLines := make(chan parser.RawLine, 1000)
	entries  := make(chan parser.LogEntry, 1000)
	alerts   := make(chan rules.Alert, 100)

	// Stdin reader — signals done when stdin closes
	stdinDone := make(chan struct{})
	go func() {
		watcher.ReadStdin(source, rawLines)
		close(rawLines)
		close(stdinDone)
	}()

	go parser.Run(rawLines, entries)
	go rules.Match(cfg.Rules, entries, alerts)
	go dispatcher.Dispatch(cfg, alerts, stats, broker)

	if showDash {
		if daemonRunning {
			// Already set up above — nothing more to do here
		} else {
			dash := dashboard.NewServer(cfg.DashboardAddr, stats, broker)
			go func() {
				if err := dash.Listen(); err != nil {
					log.Printf("dashboard: %v", err)
				}
			}()
			log.Printf("dashboard: http://localhost%s", cfg.DashboardAddr)
		}
	}

	log.Printf("logitcat pipe: reading stdin as %q — %d rule(s) active", source, len(cfg.Rules))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-stdinDone:
		time.Sleep(400 * time.Millisecond)
		if forwardCh != nil {
			broker.Unsubscribe(forwardCh)
			select {
			case <-forwardDone:
			case <-time.After(2 * time.Second):
			}
		}
		log.Printf("logitcat pipe: done. %d alert(s) fired.", stats.AlertCount())
	case <-quit:
		log.Println("logitcat pipe: interrupted.")
	}
}

// cmdRun starts the full pipeline — either in foreground (dev) or as the daemon process.
func cmdRun(isDaemon bool) {
	if len(os.Args) < 3 {
		fatalf("usage: logitcat run <config.ini>")
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

	log.Printf("logitcat started — watching %d file(s), %d rule(s) active",
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

	log.Println("logitcat stopped.")
	srv.Close()
}

// forwardAlertsToDaemon subscribes to the local broker and POSTs each event
// to the already-running daemon's /api/alert endpoint so all SSE clients
// (IDE plugins, dashboard) see alerts from both the daemon and pipe mode.
func forwardAlertsToDaemon(addr string, broker *control.Broker) {
	ch := broker.Subscribe()
	// Do NOT defer Unsubscribe — broker.Close() will close ch for us.
	for event := range ch {
		dashboard.ForwardAlert(addr, event)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
