package parser

import (
	"testing"
	"time"
)

// ── format detection ──────────────────────────────────────────────────────────

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		line string
		want Format
	}{
		{`{"level":"error","msg":"oops"}`, FormatJSON},
		{`Mar 13 11:45:12 web-01 nginx[123]: failed`, FormatSyslog},
		{`127.0.0.1 - - [13/Mar/2026:11:45:12 +0300] "GET / HTTP/1.1" 200 512`, FormatApache},
		{`plain text log line`, FormatPlaintext},
		{``, FormatPlaintext},
		{`{invalid json`, FormatJSON}, // detect() checks first char only; parseJSON handles fallback
	}

	for _, tc := range cases {
		got := detect(tc.line)
		if got != tc.want {
			t.Errorf("detect(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}

// ── JSON parser ───────────────────────────────────────────────────────────────

func TestParseJSON(t *testing.T) {
	t.Run("standard fields", func(t *testing.T) {
		raw := RawLine{
			Source: "/var/log/app.log",
			Line:   `{"level":"error","msg":"db timeout","service":"payments","duration_ms":823}`,
		}
		e := Parse(raw)

		assertEqual(t, "error", e.Level)
		assertEqual(t, "db timeout", e.Message)
		assertEqual(t, string(FormatJSON), string(e.Format))
		assertEqual(t, "/var/log/app.log", e.Source)
		assertEqual(t, "payments", e.Fields["service"])
	})

	t.Run("alternate field names", func(t *testing.T) {
		raw := RawLine{Line: `{"severity":"warn","text":"high memory","ts":"2026-03-13T11:45:12Z"}`}
		e := Parse(raw)
		assertEqual(t, "warn", e.Level)
		assertEqual(t, "high memory", e.Message)
		if e.Time.IsZero() {
			t.Error("expected parsed timestamp, got zero")
		}
	})

	t.Run("level normalisation", func(t *testing.T) {
		cases := map[string]string{
			"CRITICAL": "fatal",
			"WARNING":  "warn",
			"ERR":      "error",
			"TRACE":    "debug",
			"INFO":     "info",
		}
		for input, want := range cases {
			raw := RawLine{Line: `{"level":"` + input + `","msg":"test"}`}
			e := Parse(raw)
			if e.Level != want {
				t.Errorf("level %q → got %q, want %q", input, e.Level, want)
			}
		}
	})

	t.Run("invalid JSON falls back to plaintext", func(t *testing.T) {
		raw := RawLine{Line: `{not valid json`}
		e := Parse(raw)
		assertEqual(t, string(FormatPlaintext), string(e.Format))
	})

	t.Run("missing msg field uses raw line", func(t *testing.T) {
		raw := RawLine{Line: `{"level":"info","code":200}`}
		e := Parse(raw)
		if e.Message == "" {
			t.Error("expected non-empty message fallback")
		}
	})
}

// ── syslog parser ─────────────────────────────────────────────────────────────

func TestParseSyslog(t *testing.T) {
	t.Run("standard syslog line", func(t *testing.T) {
		raw := RawLine{Line: "Mar 13 11:45:12 web-01 nginx[1234]: authentication failure for user root"}
		e := Parse(raw)

		assertEqual(t, string(FormatSyslog), string(e.Format))
		assertEqual(t, "web-01", e.Fields["hostname"])
		assertEqual(t, "nginx", e.Fields["app"])
		assertEqual(t, "1234", e.Fields["pid"])
		assertEqual(t, "authentication failure for user root", e.Message)
		assertEqual(t, "info", e.Level) // no error keyword in message → info
	})

	t.Run("syslog without pid", func(t *testing.T) {
		raw := RawLine{Line: "Mar 13 11:45:12 db-01 postgres: LOG: database system is ready"}
		e := Parse(raw)
		assertEqual(t, string(FormatSyslog), string(e.Format))
		if _, ok := e.Fields["pid"]; ok {
			t.Error("expected no pid field for syslog without pid")
		}
	})

	t.Run("syslog timestamp gets current year", func(t *testing.T) {
		raw := RawLine{Line: "Mar 13 11:45:12 host app[1]: msg"}
		e := Parse(raw)
		if e.Time.Year() != time.Now().Year() {
			t.Errorf("expected year %d, got %d", time.Now().Year(), e.Time.Year())
		}
	})
}

// ── apache parser ─────────────────────────────────────────────────────────────

func TestParseApache(t *testing.T) {
	t.Run("5xx maps to error level", func(t *testing.T) {
		raw := RawLine{Line: `192.168.1.1 - frank [13/Mar/2026:11:45:12 +0300] "POST /api HTTP/1.1" 500 1234`}
		e := Parse(raw)
		assertEqual(t, string(FormatApache), string(e.Format))
		assertEqual(t, "error", e.Level)
		assertEqual(t, "500", e.Fields["status"])
		assertEqual(t, "192.168.1.1", e.Fields["ip"])
		assertEqual(t, "POST", e.Fields["method"])
		assertEqual(t, "/api", e.Fields["path"])
	})

	t.Run("4xx maps to warn level", func(t *testing.T) {
		raw := RawLine{Line: `10.0.0.1 - - [13/Mar/2026:11:45:12 +0300] "GET /missing HTTP/1.1" 404 0`}
		e := Parse(raw)
		assertEqual(t, "warn", e.Level)
	})

	t.Run("2xx maps to info level", func(t *testing.T) {
		raw := RawLine{Line: `10.0.0.1 - - [13/Mar/2026:11:45:12 +0300] "GET /health HTTP/1.1" 200 42`}
		e := Parse(raw)
		assertEqual(t, "info", e.Level)
	})
}

// ── plaintext parser ──────────────────────────────────────────────────────────

func TestParsePlaintext(t *testing.T) {
	cases := []struct {
		line      string
		wantLevel string
	}{
		{"2026-03-13 ERROR database connection failed", "error"},
		{"2026-03-13 WARN high memory usage", "warn"},
		{"2026-03-13 FATAL kernel panic", "fatal"},
		{"2026-03-13 DEBUG verbose output", "debug"},
		{"2026-03-13 INFO service started", "info"},
		{"just a plain line", "info"},
	}

	for _, tc := range cases {
		raw := RawLine{Line: tc.line}
		e := Parse(raw)
		assertEqual(t, string(FormatPlaintext), string(e.Format))
		if e.Level != tc.wantLevel {
			t.Errorf("line %q: level = %q, want %q", tc.line, e.Level, tc.wantLevel)
		}
	}
}

// ── level normalisation ───────────────────────────────────────────────────────

func TestNormaliseLevel(t *testing.T) {
	cases := map[string]string{
		"FATAL":    "fatal",
		"critical": "fatal",
		"CRIT":     "fatal",
		"PANIC":    "fatal",
		"ERROR":    "error",
		"ERR":      "error",
		"WARNING":  "warn",
		"WARN":     "warn",
		"DEBUG":    "debug",
		"TRACE":    "debug",
		"VERBOSE":  "debug",
		"INFO":     "info",
		"unknown":  "info",
	}
	for input, want := range cases {
		got := normaliseLevel(input)
		if got != want {
			t.Errorf("normaliseLevel(%q) = %q, want %q", input, got, want)
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertEqual(t *testing.T, want, got string) {
	t.Helper()
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
