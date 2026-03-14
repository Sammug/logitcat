package dispatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/sammug/logitcat/internal/rules"
)

// sendTeams posts an Adaptive Card to a Microsoft Teams Incoming Webhook.
func sendTeams(url string, a rules.Alert) {
	if url == "" {
		log.Println("dispatcher: teams_webhook_url not set in [output]")
		return
	}

	payload := teamsPayload(a)
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("dispatcher: teams marshal: %v", err)
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("dispatcher: teams webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("dispatcher: teams webhook returned %d", resp.StatusCode)
	}
}

// teamsPayload builds a Teams-compatible Adaptive Card message.
func teamsPayload(a rules.Alert) map[string]any {
	colour := teamsColour(a.Severity)
	icon := severityIcon(a.Severity)

	return map[string]any{
		"type": "message",
		"attachments": []map[string]any{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]any{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": "1.4",
					"body": []map[string]any{
						// Header bar with severity colour
						{
							"type":  "ColumnSet",
							"style": colour,
							"bleed": true,
							"columns": []map[string]any{
								{
									"type":  "Column",
									"width": "stretch",
									"items": []map[string]any{
										{
											"type":   "TextBlock",
											"text":   fmt.Sprintf("%s  **logitcat alert**", icon),
											"size":   "medium",
											"weight": "bolder",
											"color":  "light",
										},
									},
								},
							},
						},
						// Rule + severity
						{
							"type": "FactSet",
							"facts": []map[string]string{
								{"title": "Rule", "value": a.RuleName},
								{"title": "Severity", "value": a.Severity},
								{"title": "Level", "value": a.Entry.Level},
								{"title": "Format", "value": string(a.Entry.Format)},
								{"title": "Source", "value": a.Entry.Source},
								{"title": "Time", "value": a.Time.Format(time.RFC3339)},
							},
						},
						// Message body
						{
							"type":      "TextBlock",
							"text":      a.Entry.Message,
							"wrap":      true,
							"separator": true,
							"fontType":  "Monospace",
						},
					},
				},
			},
		},
	}
}

// teamsColour maps severity to an Adaptive Card container style.
func teamsColour(severity string) string {
	switch severity {
	case "CRITICAL":
		return "attention" // red
	case "WARN":
		return "warning" // yellow
	default:
		return "good" // green
	}
}
