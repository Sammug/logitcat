package parser

import "strings"

// normaliseLevel maps raw level strings from various libraries to a
// canonical lowercase token: fatal | error | warn | info | debug.
func normaliseLevel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "fatal", "critical", "crit", "emerg", "alert", "panic":
		return "fatal"
	case "error", "err":
		return "error"
	case "warning", "warn":
		return "warn"
	case "debug", "trace", "verbose":
		return "debug"
	default:
		return "info"
	}
}
