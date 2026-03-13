package parser

import (
	"encoding/json"
	"time"
)

// Well-known field name variants used by popular logging libraries.
var jsonLevelKeys = []string{"level", "severity", "lvl", "log.level"}
var jsonMsgKeys = []string{"message", "msg", "text", "body"}
var jsonTimeKeys = []string{"time", "timestamp", "ts", "@timestamp", "datetime"}

// Common time formats emitted by logging libraries.
var jsonTimeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006/01/02 15:04:05",
}

// parseJSON unmarshals a JSON log line into a LogEntry.
// Unknown fields are preserved in Fields as strings.
func parseJSON(line, source string) LogEntry {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		// Not valid JSON despite looking like it — fall back.
		return asPlaintext(line, source)
	}

	entry := LogEntry{
		Raw:    line,
		Source: source,
		Time:   time.Now(),
		Fields: make(map[string]string, len(raw)),
		Format: FormatJSON,
	}

	// Extract well-known fields, then store everything else as strings.
	for k, v := range raw {
		str := anyToString(v)

		if entry.Level == "" && containsKey(jsonLevelKeys, k) {
			entry.Level = normaliseLevel(str)
			continue
		}
		if entry.Message == "" && containsKey(jsonMsgKeys, k) {
			entry.Message = str
			continue
		}
		if entry.Time.IsZero() && containsKey(jsonTimeKeys, k) {
			if t, ok := parseTime(str); ok {
				entry.Time = t
				continue
			}
		}
		entry.Fields[k] = str
	}

	if entry.Level == "" {
		entry.Level = detectLevel(entry.Message)
	}
	if entry.Message == "" {
		entry.Message = line
	}

	return entry
}

// ── helpers ──────────────────────────────────────────────────────────────────

func containsKey(keys []string, target string) bool {
	for _, k := range keys {
		if k == target {
			return true
		}
	}
	return false
}

func parseTime(s string) (time.Time, bool) {
	for _, layout := range jsonTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// Avoid scientific notation for integers.
		if t == float64(int64(t)) {
			return string(rune(0)) // reuse json below
		}
	}
	b, _ := json.Marshal(v)
	return string(b)
}
