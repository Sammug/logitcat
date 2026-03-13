// Package config loads and validates logwatch configuration from an INI file.
package config

import (
	"strings"

	"gopkg.in/ini.v1"
)

// Rule describes a single alert rule.
type Rule struct {
	Name     string   // unique rule identifier
	Pattern  string   // regex to match against the target field
	Field    string   // log field to match against (default: message)
	Level    string   // minimum log level to consider (default: all)
	Severity string   // alert severity: INFO | WARN | CRITICAL
	Action   []string // output targets: stdout | file | webhook | teams | email
	Cooldown int      // minimum seconds between repeat alerts (0 = no limit)
}

// SMTPConfig holds email delivery settings.
type SMTPConfig struct {
	Host     string
	Port     int      // 587 = STARTTLS (default), 465 = TLS, 25 = plain
	User     string
	Password string
	From     string
	To       []string
}

// Config is the validated runtime configuration.
type Config struct {
	Files           []string
	Rules           []Rule
	WebhookURL      string // Slack-compatible webhook URL
	TeamsWebhookURL string // Microsoft Teams incoming webhook URL
	AlertFile       string // path to write alert log
	SMTP            SMTPConfig
}

// Load reads and validates the INI file at path.
func Load(path string) (*Config, error) {
	f, err := ini.Load(path)
	if err != nil {
		return nil, err
	}

	c := &Config{}

	if sec, err := f.GetSection("watch"); err == nil {
		for _, raw := range strings.Split(sec.Key("files").String(), ",") {
			if p := strings.TrimSpace(raw); p != "" {
				c.Files = append(c.Files, p)
			}
		}
	}

	if sec, err := f.GetSection("output"); err == nil {
		c.WebhookURL      = sec.Key("webhook_url").String()
		c.TeamsWebhookURL = sec.Key("teams_webhook_url").String()
		c.AlertFile       = sec.Key("alert_file").String()
		c.SMTP = SMTPConfig{
			Host:     sec.Key("smtp_host").String(),
			Port:     sec.Key("smtp_port").MustInt(587),
			User:     sec.Key("smtp_user").String(),
			Password: sec.Key("smtp_password").String(),
			From:     sec.Key("smtp_from").MustString("logwatch <noreply@logwatch>"),
			To:       splitTrim(sec.Key("smtp_to").String(), ","),
		}
	}

	for _, sec := range f.Sections() {
		if !strings.HasPrefix(sec.Name(), "rule:") {
			continue
		}
		c.Rules = append(c.Rules, Rule{
			Name:     strings.TrimPrefix(sec.Name(), "rule:"),
			Pattern:  sec.Key("pattern").String(),
			Field:    sec.Key("field").MustString(""),
			Level:    sec.Key("level").MustString(""),
			Severity: sec.Key("severity").MustString("INFO"),
			Action:   splitTrim(sec.Key("action").String(), ","),
			Cooldown: sec.Key("cooldown").MustInt(0),
		})
	}

	return c, nil
}

func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
