package watcher

import (
	"bufio"
	"io"
	"log"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watch tails multiple log files and sends new lines to the lines channel.
func Watch(files []string, lines chan<- string) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("watcher: failed to create: %v", err)
	}
	defer w.Close()

	for _, f := range files {
		if err := w.Add(f); err != nil {
			log.Printf("watcher: cannot watch %s: %v", f, err)
			continue
		}
		go tailFile(f, lines)
		log.Printf("watcher: watching %s", f)
	}

	for {
		select {
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) {
				// handled by tailFile goroutine
				_ = event
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

// tailFile reads new lines appended to a file (like `tail -f`).
func tailFile(path string, lines chan<- string) {
	f, err := os.Open(path)
	if err != nil {
		log.Printf("tailFile: cannot open %s: %v", path, err)
		return
	}
	defer f.Close()

	// Seek to end so we only catch new lines
	f.Seek(0, io.SeekEnd)

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		lines <- line
	}
}
