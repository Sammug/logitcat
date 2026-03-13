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

func Dispatch(cfg *config.Config, alerts <-chan rules.Alert) {
	for alert := range alerts {
		for _, action := range alert.Actions {
			switch action {
			case "stdout":
				printAlert(alert)
			case "file":
				writeToFile(cfg.AlertFile, alert)
			case "webhook":
				sendWebhook(cfg.WebhookURL, alert)
			default:
				log.Printf("dispatcher: unknown action '%s'", action)
			}
		}
	}
}

func printAlert(a rules.Alert) {
	fmt.Printf("[%s] [%s] [%s] %s",
		a.Time.Format(time.RFC3339),
		a.Severity,
		a.RuleName,
		a.Line,
	)
}

func writeToFile(path string, a rules.Alert) {
	if path == "" {
		log.Println("dispatcher: alert_file not configured")
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("dispatcher: cannot open alert file: %v", err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] [%s] [%s] %s\n",
		a.Time.Format(time.RFC3339),
		a.Severity,
		a.RuleName,
		a.Line,
	)
}

func sendWebhook(url string, a rules.Alert) {
	if url == "" {
		log.Println("dispatcher: webhook_url not configured")
		return
	}
	payload := map[string]string{
		"text": fmt.Sprintf("🚨 *[%s]* `%s`\n```%s```", a.Severity, a.RuleName, a.Line),
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("dispatcher: webhook failed: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("dispatcher: webhook sent [%d]", resp.StatusCode)
}
