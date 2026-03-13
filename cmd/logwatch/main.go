package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/sammug/logwatch/config"
	"github.com/sammug/logwatch/internal/dispatcher"
	"github.com/sammug/logwatch/internal/rules"
	"github.com/sammug/logwatch/internal/watcher"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: logwatch <config.ini>")
		os.Exit(1)
	}

	cfg, err := config.Load(os.Args[1])
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	lines := make(chan string, 1000)
	alerts := make(chan rules.Alert, 100)

	// Start pipeline
	go watcher.Watch(cfg.Files, lines)
	go rules.Match(cfg.Rules, lines, alerts)
	go dispatcher.Dispatch(cfg, alerts)

	log.Printf("logwatch started. Watching %d file(s), %d rule(s) loaded.", len(cfg.Files), len(cfg.Rules))

	// Wait for Ctrl+C
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down.")
}
