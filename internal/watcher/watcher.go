// Package watcher tails one or more log files and emits new lines as they arrive.
// It transparently handles log rotation (rename and copytruncate strategies).
package watcher

import (
	"bufio"
	"io"
	"log"
	"os"
	"time"

	"github.com/sammug/logitcat/internal/parser"
)

const (
	pollInterval    = 100 * time.Millisecond // how often to check for new data
	reopenTimeout   = 30 * time.Second       // how long to wait for rotated file to reappear
	reopenInterval  = 200 * time.Millisecond // poll interval while waiting for new file
)

// Watch launches one tailer goroutine per file.
// Each tailer survives log rotations automatically.
func Watch(files []string, out chan<- parser.RawLine) {
	for _, f := range files {
		go tailLoop(f, out)
		log.Printf("watcher: tailing %s", f)
	}
}

// tailLoop continuously tails path, reopening the file transparently after rotations.
func tailLoop(path string, out chan<- parser.RawLine) {
	seekEnd := true // first open: start from EOF (don't replay history)
	for {
		rotated, err := tailUntilRotation(path, seekEnd, out)
		if err != nil {
			// File doesn't exist or couldn't be opened — wait for it.
			log.Printf("watcher: %s unavailable (%v), waiting...", path, err)
			if !waitForFile(path, reopenTimeout) {
				log.Printf("watcher: %s did not reappear within timeout, retrying", path)
			}
			seekEnd = false // read new file from the start after waiting
			continue
		}
		if rotated {
			log.Printf("watcher: %s rotated — waiting for new file", path)
			if !waitForFile(path, reopenTimeout) {
				log.Printf("watcher: %s did not reappear within timeout, retrying", path)
			}
			log.Printf("watcher: %s reopened after rotation", path)
			seekEnd = false // read rotated file from the beginning
		}
	}
}

// tailUntilRotation tails path, forwarding new lines to out.
// seekEnd=true starts from EOF (initial open); false reads from the beginning (after rotation).
// It returns (true, nil) when rotation is detected, (false, err) on open failure.
func tailUntilRotation(path string, seekEnd bool, out chan<- parser.RawLine) (rotated bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Snapshot the file identity at open time for rotation detection.
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}

	// Seek to EOF on first open (don't replay history), or start of file after rotation.
	var offset int64
	if seekEnd {
		offset, err = f.Seek(0, io.SeekEnd)
	} else {
		offset, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		return false, err
	}

	r := bufio.NewReader(f)

	for {
		line, readErr := r.ReadString('\n')

		if len(line) > 0 {
			out <- parser.RawLine{Source: path, Line: line}
			// Update our tracked offset.
			offset += int64(len(line))
		}

		if readErr == nil {
			// More data may be available immediately — don't sleep.
			continue
		}

		// No new data. Check if the file has been rotated.
		if yes, reason := checkRotation(path, fi, offset); yes {
			log.Printf("watcher: rotation detected for %s (%s)", path, reason)
			return true, nil
		}

		time.Sleep(pollInterval)
	}
}

// checkRotation returns (true, reason) if the file at path has been rotated
// relative to the original FileInfo fi at the tracked read offset.
func checkRotation(path string, fi os.FileInfo, offset int64) (bool, string) {
	current, err := os.Stat(path)
	if err != nil {
		// File is gone — rotation in progress.
		return true, "file removed"
	}

	// Different underlying file (rename rotation — new inode).
	if !os.SameFile(fi, current) {
		return true, "inode changed"
	}

	// File is smaller than our read position (copytruncate rotation).
	if current.Size() < offset {
		return true, "file truncated"
	}

	return false, ""
}

// waitForFile blocks until path exists and is readable, or the timeout elapses.
// Returns true if the file appeared, false if the timeout was hit.
func waitForFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			// Give the writer a moment to start writing to the new file.
			time.Sleep(reopenInterval)
			return true
		}
		time.Sleep(reopenInterval)
	}
	return false
}
