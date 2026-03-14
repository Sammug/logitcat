package rules

import (
	"testing"
	"time"

	"github.com/sammug/logitcat/config"
	"github.com/sammug/logitcat/internal/parser"
)

// makeEntry is a helper to build a LogEntry for tests.
func makeEntry(level, message string, fields map[string]string) parser.LogEntry {
	if fields == nil {
		fields = map[string]string{}
	}
	return parser.LogEntry{
		Raw:     message,
		Source:  "/var/log/test.log",
		Time:    time.Now(),
		Level:   level,
		Message: message,
		Fields:  fields,
		Format:  parser.FormatPlaintext,
	}
}

// runMatch feeds entries into Match and collects the alerts produced.
func runMatch(rules []config.Rule, entries []parser.LogEntry) []Alert {
	in := make(chan parser.LogEntry, len(entries))
	out := make(chan Alert, len(entries)*len(rules))

	for _, e := range entries {
		in <- e
	}
	close(in)

	Match(rules, in, out)
	close(out)

	var alerts []Alert
	for a := range out {
		alerts = append(alerts, a)
	}
	return alerts
}

// ── basic matching ────────────────────────────────────────────────────────────

func TestBasicPatternMatch(t *testing.T) {
	rules := []config.Rule{
		{Name: "errors", Pattern: `ERROR|FATAL`, Severity: "CRITICAL", Action: []string{"stdout"}},
	}

	entries := []parser.LogEntry{
		makeEntry("error", "ERROR: database down", nil),
		makeEntry("info", "INFO: all good", nil),
		makeEntry("fatal", "FATAL: disk full", nil),
	}

	alerts := runMatch(rules, entries)
	if len(alerts) != 2 {
		t.Errorf("expected 2 alerts, got %d", len(alerts))
	}
}

func TestNoMatchWhenPatternDoesNotMatch(t *testing.T) {
	rules := []config.Rule{
		{Name: "r", Pattern: `CRITICAL`, Severity: "CRITICAL", Action: []string{"stdout"}},
	}
	alerts := runMatch(rules, []parser.LogEntry{
		makeEntry("info", "everything is fine", nil),
	})
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

// ── field-specific matching ───────────────────────────────────────────────────

func TestFieldMatch(t *testing.T) {
	rules := []config.Rule{{
		Name:     "payments-errors",
		Pattern:  `payments`,
		Field:    "service",
		Severity: "CRITICAL",
		Action:   []string{"stdout"},
	}}

	entries := []parser.LogEntry{
		makeEntry("error", "timeout", map[string]string{"service": "payments"}),
		makeEntry("error", "timeout", map[string]string{"service": "auth"}),
	}

	alerts := runMatch(rules, entries)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Entry.Fields["service"] != "payments" {
		t.Errorf("wrong entry matched: service = %q", alerts[0].Entry.Fields["service"])
	}
}

func TestRawFieldMatch(t *testing.T) {
	rules := []config.Rule{{
		Name: "raw-match", Pattern: `special-token`, Field: "raw", Severity: "WARN",
		Action: []string{"stdout"},
	}}
	entry := parser.LogEntry{
		Raw: "2026-03-13 special-token detected", Message: "sanitised message",
		Fields: map[string]string{},
	}
	alerts := runMatch(rules, []parser.LogEntry{entry})
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert on raw field, got %d", len(alerts))
	}
}

// ── level filtering ───────────────────────────────────────────────────────────

func TestLevelFilter(t *testing.T) {
	rules := []config.Rule{{
		Name: "high-only", Pattern: `.*`, Level: "error", Severity: "CRITICAL",
		Action: []string{"stdout"},
	}}

	entries := []parser.LogEntry{
		makeEntry("debug", "verbose output", nil),
		makeEntry("info", "service started", nil),
		makeEntry("warn", "high memory", nil),
		makeEntry("error", "db failed", nil),
		makeEntry("fatal", "kernel panic", nil),
	}

	alerts := runMatch(rules, entries)
	if len(alerts) != 2 { // error + fatal
		t.Errorf("expected 2 alerts (error+fatal), got %d", len(alerts))
	}
}

func TestLevelFilterAllPass(t *testing.T) {
	rules := []config.Rule{{
		Name: "all", Pattern: `msg`, Level: "", Severity: "INFO",
		Action: []string{"stdout"},
	}}
	entries := []parser.LogEntry{
		makeEntry("debug", "msg", nil),
		makeEntry("info", "msg", nil),
		makeEntry("error", "msg", nil),
	}
	alerts := runMatch(rules, entries)
	if len(alerts) != 3 {
		t.Errorf("expected 3 alerts with no level filter, got %d", len(alerts))
	}
}

// ── level ordering ────────────────────────────────────────────────────────────

func TestLevelOrdering(t *testing.T) {
	cases := []struct {
		actual  string
		minimum string
		pass    bool
	}{
		{"fatal", "error", true},
		{"error", "error", true},
		{"warn", "error", false},
		{"info", "warn", false},
		{"debug", "info", false},
		{"fatal", "debug", true},
		{"unknown", "warn", true}, // unknown levels pass through
	}
	for _, tc := range cases {
		got := levelAtLeast(tc.actual, tc.minimum)
		if got != tc.pass {
			t.Errorf("levelAtLeast(%q, %q) = %v, want %v", tc.actual, tc.minimum, got, tc.pass)
		}
	}
}

// ── cooldown ──────────────────────────────────────────────────────────────────

func TestCooldown(t *testing.T) {
	rules := []config.Rule{{
		Name: "noisy", Pattern: `ERROR`, Severity: "CRITICAL",
		Action: []string{"stdout"}, Cooldown: 60, // 60 second cooldown
	}}

	// Feed 5 matching entries — only 1 alert should fire due to cooldown.
	entries := make([]parser.LogEntry, 5)
	for i := range entries {
		entries[i] = makeEntry("error", "ERROR repeated", nil)
	}

	alerts := runMatch(rules, entries)
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert with cooldown=60, got %d", len(alerts))
	}
}

func TestNoCooldown(t *testing.T) {
	rules := []config.Rule{{
		Name: "every", Pattern: `ERROR`, Severity: "CRITICAL",
		Action: []string{"stdout"}, Cooldown: 0,
	}}

	entries := make([]parser.LogEntry, 5)
	for i := range entries {
		entries[i] = makeEntry("error", "ERROR repeated", nil)
	}

	alerts := runMatch(rules, entries)
	if len(alerts) != 5 {
		t.Errorf("expected 5 alerts with no cooldown, got %d", len(alerts))
	}
}

// ── multiple rules ────────────────────────────────────────────────────────────

func TestMultipleRules(t *testing.T) {
	rules := []config.Rule{
		{Name: "errors", Pattern: `ERROR`, Severity: "CRITICAL", Action: []string{"stdout"}},
		{Name: "auth", Pattern: `auth failure`, Severity: "WARN", Action: []string{"stdout"}},
	}

	entries := []parser.LogEntry{
		makeEntry("error", "ERROR db down", nil),
		makeEntry("warn", "auth failure detected", nil),
		makeEntry("info", "all good", nil),
	}

	alerts := runMatch(rules, entries)
	if len(alerts) != 2 {
		t.Errorf("expected 2 alerts from 2 rules, got %d", len(alerts))
	}
}

func TestInvalidPatternSkipped(t *testing.T) {
	rules := []config.Rule{
		{Name: "bad", Pattern: `[invalid`, Severity: "CRITICAL", Action: []string{"stdout"}},
		{Name: "good", Pattern: `ERROR`, Severity: "CRITICAL", Action: []string{"stdout"}},
	}
	entries := []parser.LogEntry{makeEntry("error", "ERROR occurred", nil)}
	alerts := runMatch(rules, entries)
	// bad rule skipped, good rule fires
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert (bad pattern skipped), got %d", len(alerts))
	}
}

// ── alert content ─────────────────────────────────────────────────────────────

func TestAlertContents(t *testing.T) {
	rule := config.Rule{
		Name: "check", Pattern: `FATAL`, Severity: "CRITICAL",
		Action: []string{"stdout", "file"},
	}
	entry := makeEntry("fatal", "FATAL: out of memory", nil)
	alerts := runMatch([]config.Rule{rule}, []parser.LogEntry{entry})

	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert")
	}
	a := alerts[0]
	if a.RuleName != "check" {
		t.Errorf("RuleName = %q, want %q", a.RuleName, "check")
	}
	if a.Severity != "CRITICAL" {
		t.Errorf("Severity = %q, want %q", a.Severity, "CRITICAL")
	}
	if len(a.Actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(a.Actions))
	}
	if a.Time.IsZero() {
		t.Error("expected non-zero alert time")
	}
}
