package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/sammug/logwatch/config"
	"github.com/sammug/logwatch/internal/dispatcher"
	"github.com/sammug/logwatch/internal/parser"
	"github.com/sammug/logwatch/internal/rules"
	"github.com/sammug/logwatch/internal/watcher"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: logwatch <config.ini>")
		os.Exit(1)
	}

	cfg, err := config.Load(os.Args[1])
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Pipeline channels
	rawLines := make(chan parser.RawLine, 1000)
	entries  := make(chan parser.LogEntry, 1000)
	alerts   := make(chan rules.Alert, 100)

	// Start pipeline stages
	go watcher.Watch(cfg.Files, rawLines)
	go parser.Run(rawLines, entries)
	go rules.Match(cfg.Rules, entries, alerts)
	go dispatcher.Dispatch(cfg, alerts)

	log.Printf("logwatch started — watching %d file(s), %d rule(s) active",
		len(cfg.Files), len(cfg.Rules))

	// Block until SIGINT / SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("logwatch stopped.")
}
