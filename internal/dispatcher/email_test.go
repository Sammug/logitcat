package dispatcher

import (
	"strings"
	"testing"
	"time"

	"github.com/sammug/logwatch/internal/parser"
	"github.com/sammug/logwatch/internal/rules"
)

func testAlert(severity string) rules.Alert {
	return rules.Alert{
		RuleName: "test-rule",
		Severity: severity,
		Actions:  []string{"email"},
		Time:     time.Now(),
		Entry: parser.LogEntry{
			Raw:     "ERROR: database connection failed on host db-01",
			Source:  "/var/log/app.log",
			Level:   "error",
			Message: "ERROR: database connection failed on host db-01",
			Format:  parser.FormatPlaintext,
			Fields:  map[string]string{"service": "payments"},
		},
	}
}

// ── plain text rendering ──────────────────────────────────────────────────────

func TestRenderEmailPlain(t *testing.T) {
	a := testAlert("CRITICAL")
	plain := renderEmailPlain(a)

	checks := []string{"test-rule", "CRITICAL", "error", "/var/log/app.log", "database connection failed"}
	for _, want := range checks {
		if !strings.Contains(plain, want) {
			t.Errorf("plain email missing %q\n%s", want, plain)
		}
	}
}

// ── HTML rendering ────────────────────────────────────────────────────────────

func TestRenderEmailHTML(t *testing.T) {
	a := testAlert("CRITICAL")
	html, err := renderEmailHTML(a)
	if err != nil {
		t.Fatalf("renderEmailHTML: %v", err)
	}

	checks := []string{
		"test-rule",
		"CRITICAL",
		"database connection failed",
		"#d93025",         // CRITICAL red
		"/var/log/app.log",
		"logwatch alert",
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Errorf("HTML email missing %q", want)
		}
	}
}

func TestHTMLSeverityColours(t *testing.T) {
	cases := map[string]string{
		"CRITICAL": "#d93025",
		"WARN":     "#f09d00",
		"INFO":     "#1a73e8",
	}
	for severity, wantColour := range cases {
		html, err := renderEmailHTML(testAlert(severity))
		if err != nil {
			t.Fatalf("renderEmailHTML(%s): %v", severity, err)
		}
		if !strings.Contains(html, wantColour) {
			t.Errorf("severity %q: expected colour %q in HTML", severity, wantColour)
		}
	}
}

// ── MIME structure ────────────────────────────────────────────────────────────

func TestBuildMIME(t *testing.T) {
	msg := buildMIME(
		"logwatch <alerts@example.com>",
		[]string{"admin@example.com", "ops@example.com"},
		"[logwatch] [CRITICAL] test-rule triggered",
		"plain text body",
		"<html>html body</html>",
	)
	raw := string(msg)

	checks := []string{
		"From: logwatch <alerts@example.com>",
		"To: admin@example.com, ops@example.com",
		"Subject: [logwatch] [CRITICAL] test-rule triggered",
		"MIME-Version: 1.0",
		"multipart/alternative",
		"text/plain",
		"text/html",
		"plain text body",
		"<html>html body</html>",
	}
	for _, want := range checks {
		if !strings.Contains(raw, want) {
			t.Errorf("MIME message missing %q", want)
		}
	}
}

// ── colour helper ─────────────────────────────────────────────────────────────

func TestEmailColour(t *testing.T) {
	if emailColour("CRITICAL") != "#d93025" {
		t.Error("CRITICAL should be red")
	}
	if emailColour("WARN") != "#f09d00" {
		t.Error("WARN should be yellow")
	}
	if emailColour("INFO") != "#1a73e8" {
		t.Error("INFO should be blue")
	}
	if emailColour("UNKNOWN") != "#1a73e8" {
		t.Error("unknown should fall back to blue")
	}
}
