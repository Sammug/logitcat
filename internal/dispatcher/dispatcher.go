// Package dispatcher routes Alerts to one or more output targets.
package dispatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/sammug/logwatch/config"
	"github.com/sammug/logwatch/internal/rules"
)

// Dispatch reads alerts and fans each one out to its configured targets.
func Dispatch(cfg *config.Config, alerts <-chan rules.Alert) {
	for a := range alerts {
		for _, action := range a.Actions {
			switch action {
			case "stdout":
				printAlert(a)
			case "file":
				writeFile(cfg.AlertFile, a)
			case "webhook":
				sendWebhook(cfg.WebhookURL, a)
			default:
				log.Printf("dispatcher: unknown action %q in rule %q", action, a.RuleName)
			}
		}
	}
}

// ── output handlers ──────────────────────────────────────────────────────────

func printAlert(a rules.Alert) {
	fmt.Printf("[%s] [%-8s] [%s] %s\n",
		a.Time.Format(time.RFC3339),
		a.Severity,
		a.RuleName,
		a.Entry.Message,
	)
}

func writeFile(path string, a rules.Alert) {
	if path == "" {
		log.Println("dispatcher: alert_file not set in [output]")
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("dispatcher: open alert file: %v", err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "[%s] [%-8s] [%s] %s\n",
		a.Time.Format(time.RFC3339),
		a.Severity,
		a.RuleName,
		a.Entry.Message,
	)
}

func sendWebhook(url string, a rules.Alert) {
	if url == "" {
		log.Println("dispatcher: webhook_url not set in [output]")
		return
	}

	icon := severityIcon(a.Severity)
	payload := map[string]any{
		"text": fmt.Sprintf("%s *[%s]* `%s`\n```%s```",
			icon, a.Severity, a.RuleName, a.Entry.Message),
		"attachments": []map[string]any{{
			"color": severityColour(a.Severity),
			"fields": []map[string]string{
				{"title": "Source", "value": a.Entry.Source, "short": "true"},
				{"title": "Level", "value": a.Entry.Level, "short": "true"},
				{"title": "Format", "value": string(a.Entry.Format), "short": "true"},
				{"title": "Time", "value": a.Entry.Time.Format(time.RFC3339), "short": "true"},
			},
		}},
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("dispatcher: webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("dispatcher: webhook returned %d", resp.StatusCode)
	}
}

func severityIcon(s string) string {
	switch s {
	case "CRITICAL":
		return "🚨"
	case "WARN":
		return "⚠️"
	default:
		return "ℹ️"
	}
}

func severityColour(s string) string {
	switch s {
	case "CRITICAL":
		return "danger"
	case "WARN":
		return "warning"
	default:
		return "good"
	}
}
