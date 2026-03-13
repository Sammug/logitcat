package parser

import (
	"regexp"
	"strings"
	"time"
)

// reSyslog matches the standard syslog format:
// Mar 13 11:45:12 hostname app[pid]: message
// or with year: 2026-03-13T11:45:12 hostname app[pid]: message
var reSyslog = regexp.MustCompile(
	`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}|\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})\s+(\S+)\s+(\S+?)(?:\[(\d+)\])?:\s+(.+)$`,
)

var syslogTimeFormats = []string{
	"Jan  2 15:04:05",
	"Jan 2 15:04:05",
	"2006-01-02T15:04:05",
}

// parseSyslog parses a syslog-formatted log line.
func parseSyslog(line, source string) LogEntry {
	m := reSyslog.FindStringSubmatch(line)
	if m == nil {
		return asPlaintext(line, source)
	}
	// m[1]=timestamp  m[2]=hostname  m[3]=app  m[4]=pid (optional)  m[5]=message

	entry := LogEntry{
		Raw:     line,
		Source:  source,
		Time:    parseSyslogTime(m[1]),
		Message: strings.TrimSpace(m[5]),
		Fields: map[string]string{
			"hostname": m[2],
			"app":      m[3],
		},
		Format: FormatSyslog,
	}
	if m[4] != "" {
		entry.Fields["pid"] = m[4]
	}

	entry.Level = detectLevel(entry.Message)
	return entry
}

func parseSyslogTime(s string) time.Time {
	for _, layout := range syslogTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			// Syslog timestamps have no year — use current year.
			if t.Year() == 0 {
				t = t.AddDate(time.Now().Year(), 0, 0)
			}
			return t
		}
	}
	return time.Now()
}
