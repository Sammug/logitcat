package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemp writes content to a temp file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "logitcat-*.ini")
	if err != nil {
		t.Fatalf("createTemp: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write: %v", err)
	}
	return f.Name()
}

func TestLoadBasicConfig(t *testing.T) {
	path := writeTemp(t, `
[watch]
files = /var/log/app.log, /var/log/nginx.log

[output]
alert_file        = /tmp/alerts.log
webhook_url       = https://hooks.slack.com/xxx
teams_webhook_url = https://teams.example.com/xxx

[rule:errors]
pattern  = ERROR|FATAL
severity = CRITICAL
action   = stdout, file, webhook
cooldown = 30
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Files
	if len(cfg.Files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(cfg.Files), cfg.Files)
	}

	// Output
	if cfg.AlertFile != "/tmp/alerts.log" {
		t.Errorf("AlertFile = %q", cfg.AlertFile)
	}
	if cfg.WebhookURL != "https://hooks.slack.com/xxx" {
		t.Errorf("WebhookURL = %q", cfg.WebhookURL)
	}
	if cfg.TeamsWebhookURL != "https://teams.example.com/xxx" {
		t.Errorf("TeamsWebhookURL = %q", cfg.TeamsWebhookURL)
	}

	// Rules
	if len(cfg.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(cfg.Rules))
	}
	r := cfg.Rules[0]
	if r.Name != "errors" {
		t.Errorf("Name = %q, want %q", r.Name, "errors")
	}
	if r.Pattern != "ERROR|FATAL" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if r.Severity != "CRITICAL" {
		t.Errorf("Severity = %q", r.Severity)
	}
	if r.Cooldown != 30 {
		t.Errorf("Cooldown = %d, want 30", r.Cooldown)
	}
	if len(r.Action) != 3 {
		t.Errorf("expected 3 actions, got %v", r.Action)
	}
}

func TestLoadMultipleRules(t *testing.T) {
	path := writeTemp(t, `
[watch]
files = /tmp/test.log

[rule:auth]
pattern  = auth failure
severity = WARN
action   = stdout
cooldown = 10

[rule:disk]
pattern  = No space left
severity = CRITICAL
action   = stdout, webhook
cooldown = 60
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(cfg.Rules))
	}
}

func TestLoadFieldAndLevelOptions(t *testing.T) {
	path := writeTemp(t, `
[watch]
files = /tmp/test.log

[rule:payments]
pattern  = payments
field    = service
level    = error
severity = CRITICAL
action   = stdout
cooldown = 0
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := cfg.Rules[0]
	if r.Field != "service" {
		t.Errorf("Field = %q, want %q", r.Field, "service")
	}
	if r.Level != "error" {
		t.Errorf("Level = %q, want %q", r.Level, "error")
	}
}

func TestLoadDefaultSeverity(t *testing.T) {
	path := writeTemp(t, `
[watch]
files = /tmp/test.log

[rule:simple]
pattern = ERROR
action  = stdout
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Rules[0].Severity != "INFO" {
		t.Errorf("default severity should be INFO, got %q", cfg.Rules[0].Severity)
	}
}

func TestLoadEmptyFiles(t *testing.T) {
	path := writeTemp(t, `
[watch]
files =

[rule:r]
pattern = x
action  = stdout
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Files) != 0 {
		t.Errorf("expected 0 files, got %v", cfg.Files)
	}
}

func TestLoadSMTPConfig(t *testing.T) {
	path := writeTemp(t, `
[watch]
files = /tmp/test.log

[output]
smtp_host     = smtp.gmail.com
smtp_port     = 587
smtp_user     = alerts@example.com
smtp_password = secret
smtp_from     = logitcat <alerts@example.com>
smtp_to       = admin@example.com, ops@example.com
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s := cfg.SMTP
	if s.Host != "smtp.gmail.com" {
		t.Errorf("Host = %q", s.Host)
	}
	if s.Port != 587 {
		t.Errorf("Port = %d, want 587", s.Port)
	}
	if s.User != "alerts@example.com" {
		t.Errorf("User = %q", s.User)
	}
	if s.Password != "secret" {
		t.Errorf("Password = %q", s.Password)
	}
	if len(s.To) != 2 {
		t.Errorf("To = %v, want 2 recipients", s.To)
	}
}

func TestLoadSMTPDefaults(t *testing.T) {
	path := writeTemp(t, `
[watch]
files = /tmp/test.log

[output]
smtp_host = smtp.example.com
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SMTP.Port != 587 {
		t.Errorf("default Port = %d, want 587", cfg.SMTP.Port)
	}
	if !strings.Contains(cfg.SMTP.From, "logitcat") {
		t.Errorf("default From = %q, want logitcat in it", cfg.SMTP.From)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexistent.ini"))
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestSplitTrim(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"stdout, file, webhook", []string{"stdout", "file", "webhook"}},
		{"stdout", []string{"stdout"}},
		{"  stdout  ,  file  ", []string{"stdout", "file"}},
		{"", []string{}},
		{"  ,  ,  ", []string{}},
	}
	for _, tc := range cases {
		got := splitTrim(tc.input, ",")
		if len(got) != len(tc.want) {
			t.Errorf("splitTrim(%q): got %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitTrim(%q)[%d]: got %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}
