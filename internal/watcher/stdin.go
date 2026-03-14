package watcher

import (
	"bufio"
	"io"
	"log"
	"os"

	"github.com/sammug/logwatch/internal/parser"
)

// ReadStdin reads lines from stdin and forwards them to out.
// source is used as the log entry source label (e.g. "adb-logcat", "gradle").
// Blocks until stdin is closed (EOF) or an error occurs.
func ReadStdin(source string, out chan<- parser.RawLine) {
	if source == "" {
		source = "stdin"
	}

	r := bufio.NewReaderSize(os.Stdin, 1024*1024) // 1 MB buffer — handles long stack traces
	log.Printf("watcher: reading from stdin (source=%s)", source)

	for {
		line, err := r.ReadString('\n')

		if len(line) > 0 {
			out <- parser.RawLine{Source: source, Line: line}
		}

		if err != nil {
			if err == io.EOF {
				log.Printf("watcher: stdin closed (EOF)")
			} else {
				log.Printf("watcher: stdin error: %v", err)
			}
			return
		}
	}
}
