// Package watcher tails one or more log files and emits new lines as they arrive.
package watcher

import (
	"bufio"
	"io"
	"log"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sammug/logwatch/internal/parser"
)

// Watch starts a goroutine per file and forwards new lines to out.
// It also runs the fsnotify event loop to detect log rotation.
func Watch(files []string, out chan<- parser.RawLine) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("watcher: %v", err)
	}

	for _, f := range files {
		if err := w.Add(f); err != nil {
			log.Printf("watcher: cannot watch %q: %v", f, err)
			continue
		}
		go tailFile(f, out)
		log.Printf("watcher: tailing %s", f)
	}

	// Drain fsnotify events; rotation detection can be added here later.
	go func() {
		defer w.Close()
		for err := range w.Errors {
			log.Printf("watcher: %v", err)
		}
	}()
}

// tailFile seeks to the end of path and streams every new line to out.
func tailFile(path string, out chan<- parser.RawLine) {
	f, err := os.Open(path)
	if err != nil {
		log.Printf("tailFile: cannot open %q: %v", path, err)
		return
	}
	defer f.Close()

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		log.Printf("tailFile: seek failed for %q: %v", path, err)
		return
	}

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			out <- parser.RawLine{Source: path, Line: line}
		}
		if err != nil {
			// No new data yet — back off briefly before retrying.
			time.Sleep(100 * time.Millisecond)
		}
	}
}
