// Package parser detects and parses log line formats into structured LogEntry values.
// Supported formats: JSON, syslog, Apache/Nginx access logs, plaintext fallback.
package parser

import (
	"strings"
	"time"
)

// Format identifies the detected log line format.
type Format string

const (
	FormatJSON      Format = "json"
	FormatSyslog    Format = "syslog"
	FormatApache    Format = "apache"
	FormatPlaintext Format = "plaintext"
)

// RawLine carries a log line together with its source file path.
// This is what the watcher emits downstream.
type RawLine struct {
	Source string
	Line   string
}

// LogEntry is a fully structured log line ready for rule matching.
type LogEntry struct {
	Raw     string            // original, unmodified line
	Source  string            // file the line came from
	Time    time.Time         // parsed or observed timestamp
	Level   string            // normalised: error | warn | info | debug
	Message string            // primary human-readable message
	Fields  map[string]string // all additional key-value pairs
	Format  Format            // format that was detected
}

// Parse auto-detects the format of line and returns a structured LogEntry.
func Parse(raw RawLine) LogEntry {
	line := strings.TrimRight(raw.Line, "\r\n")

	switch detect(line) {
	case FormatJSON:
		return parseJSON(line, raw.Source)
	case FormatSyslog:
		return parseSyslog(line, raw.Source)
	case FormatApache:
		return parseApache(line, raw.Source)
	default:
		return asPlaintext(line, raw.Source)
	}
}

// Run is the parser stage of the pipeline.
// It reads RawLines, parses them, and forwards LogEntries to entries.
func Run(in <-chan RawLine, out chan<- LogEntry) {
	for raw := range in {
		out <- Parse(raw)
	}
}

// detect returns the most likely Format for line without allocating.
func detect(line string) Format {
	if len(line) == 0 {
		return FormatPlaintext
	}
	if line[0] == '{' {
		return FormatJSON
	}
	if reApache.MatchString(line) {
		return FormatApache
	}
	if reSyslog.MatchString(line) {
		return FormatSyslog
	}
	return FormatPlaintext
}

// asPlaintext wraps a raw line as a minimal LogEntry.
func asPlaintext(line, source string) LogEntry {
	return LogEntry{
		Raw:     line,
		Source:  source,
		Time:    time.Now(),
		Level:   detectLevel(line),
		Message: line,
		Fields:  map[string]string{},
		Format:  FormatPlaintext,
	}
}

// detectLevel does a cheap case-insensitive scan for a severity keyword.
func detectLevel(s string) string {
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "fatal") || strings.Contains(lower, "panic"):
		return "fatal"
	case strings.Contains(lower, "error") || strings.Contains(lower, "err"):
		return "error"
	case strings.Contains(lower, "warn"):
		return "warn"
	case strings.Contains(lower, "debug"):
		return "debug"
	default:
		return "info"
	}
}
