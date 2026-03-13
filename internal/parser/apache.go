package parser

import (
	"fmt"
	"regexp"
	"time"
)

// reApache matches the Combined Log Format used by Apache and Nginx:
// 127.0.0.1 - frank [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.1" 200 2326 "http://ref" "Mozilla/5.0"
var reApache = regexp.MustCompile(
	`^(\S+)\s+\S+\s+(\S+)\s+\[([^\]]+)\]\s+"(\S+)\s+(\S+)\s+(\S+)"\s+(\d{3})\s+(\S+)`,
)

const apacheTimeFormat = "02/Jan/2006:15:04:05 -0700"

// parsApache parses an Apache/Nginx Combined Log Format line.
func parseApache(line, source string) LogEntry {
	m := reApache.FindStringSubmatch(line)
	if m == nil {
		return asPlaintext(line, source)
	}
	// m[1]=ip  m[2]=user  m[3]=time  m[4]=method  m[5]=path  m[6]=proto
	// m[7]=status  m[8]=bytes

	t, _ := time.Parse(apacheTimeFormat, m[3])
	if t.IsZero() {
		t = time.Now()
	}

	status := m[7]
	entry := LogEntry{
		Raw:     line,
		Source:  source,
		Time:    t,
		Message: fmt.Sprintf("%s %s %s", m[4], m[5], status),
		Fields: map[string]string{
			"ip":     m[1],
			"user":   m[2],
			"method": m[4],
			"path":   m[5],
			"proto":  m[6],
			"status": status,
			"bytes":  m[8],
		},
		Format: FormatApache,
	}

	entry.Level = apacheLevel(status)
	return entry
}

// apacheLevel maps HTTP status codes to log levels.
func apacheLevel(status string) string {
	if len(status) == 0 {
		return "info"
	}
	switch status[0] {
	case '5':
		return "error"
	case '4':
		return "warn"
	default:
		return "info"
	}
}
