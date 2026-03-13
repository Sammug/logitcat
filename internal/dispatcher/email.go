package dispatcher

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"strings"
	"time"

	"github.com/sammug/logwatch/config"
	"github.com/sammug/logwatch/internal/rules"
)

// sendEmail sends an alert via SMTP.
// Auto-selects TLS (port 465) or STARTTLS (port 587/25) based on configured port.
func sendEmail(cfg *config.Config, a rules.Alert) {
	sc := cfg.SMTP
	if sc.Host == "" {
		log.Println("dispatcher: smtp_host not set in [output]")
		return
	}
	if len(sc.To) == 0 {
		log.Println("dispatcher: smtp_to not set in [output]")
		return
	}

	subject := fmt.Sprintf("[logwatch] [%s] %s triggered", a.Severity, a.RuleName)
	html, err := renderEmailHTML(a)
	if err != nil {
		log.Printf("dispatcher: email template: %v", err)
		return
	}
	plain := renderEmailPlain(a)

	msg := buildMIME(sc.From, sc.To, subject, plain, html)
	addr := fmt.Sprintf("%s:%d", sc.Host, sc.Port)

	var sendErr error
	if sc.Port == 465 {
		sendErr = sendTLS(addr, sc, msg)
	} else {
		sendErr = sendSTARTTLS(addr, sc, msg)
	}
	if sendErr != nil {
		log.Printf("dispatcher: email: %v", sendErr)
	}
}

// sendSTARTTLS connects plaintext then upgrades to TLS (port 587 / 25).
func sendSTARTTLS(addr string, sc config.SMTPConfig, msg []byte) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer c.Close()

	if err := c.StartTLS(&tls.Config{ServerName: sc.Host}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}
	return deliverSMTP(c, sc, msg)
}

// sendTLS opens a TLS connection directly (port 465).
func sendTLS(addr string, sc config.SMTPConfig, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: sc.Host})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	c, err := smtp.NewClient(conn, sc.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()
	return deliverSMTP(c, sc, msg)
}

// deliverSMTP authenticates, sets envelope, and writes the message body.
func deliverSMTP(c *smtp.Client, sc config.SMTPConfig, msg []byte) error {
	if sc.User != "" {
		auth := smtp.PlainAuth("", sc.User, sc.Password, sc.Host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := c.Mail(sc.From); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	for _, to := range sc.To {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", to, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// ── MIME message builder ──────────────────────────────────────────────────────

// buildMIME creates a multipart/alternative MIME email (plain + HTML).
func buildMIME(from string, to []string, subject, plain, html string) []byte {
	boundary := "logwatch-mime-boundary"
	var b bytes.Buffer

	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n", boundary)
	fmt.Fprintf(&b, "\r\n")

	// Plain text part
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	fmt.Fprintf(&b, "%s\r\n", plain)

	// HTML part
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	fmt.Fprintf(&b, "Content-Type: text/html; charset=UTF-8\r\n\r\n")
	fmt.Fprintf(&b, "%s\r\n", html)

	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes()
}

// ── templates ─────────────────────────────────────────────────────────────────

var emailHTMLTmpl = template.Must(template.New("email").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
  body{font-family:Arial,sans-serif;background:#f4f4f4;margin:0;padding:20px}
  .wrap{max-width:600px;margin:0 auto;background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 2px 6px rgba(0,0,0,.12)}
  .hdr{background:{{.HeaderColor}};color:#fff;padding:20px 24px}
  .hdr h2{margin:0;font-size:18px}
  .hdr p{margin:4px 0 0;opacity:.85;font-size:13px}
  .body{padding:24px}
  table{width:100%;border-collapse:collapse;font-size:14px}
  td{padding:9px 12px;border-bottom:1px solid #eee}
  td:first-child{font-weight:bold;color:#555;width:110px}
  .msg{background:#f8f8f8;border-left:4px solid {{.HeaderColor}};padding:14px 16px;margin-top:18px;font-family:monospace;font-size:13px;word-break:break-all;border-radius:0 4px 4px 0}
  .foot{padding:14px 24px;background:#fafafa;color:#aaa;font-size:11px;border-top:1px solid #eee}
</style>
</head>
<body>
<div class="wrap">
  <div class="hdr">
    <h2>{{.Icon}} logwatch alert</h2>
    <p>{{.RuleName}} — {{.Severity}}</p>
  </div>
  <div class="body">
    <table>
      <tr><td>Rule</td><td>{{.RuleName}}</td></tr>
      <tr><td>Severity</td><td>{{.Severity}}</td></tr>
      <tr><td>Level</td><td>{{.Level}}</td></tr>
      <tr><td>Source</td><td>{{.Source}}</td></tr>
      <tr><td>Format</td><td>{{.Format}}</td></tr>
      <tr><td>Time</td><td>{{.Time}}</td></tr>
    </table>
    <div class="msg">{{.Message}}</div>
  </div>
  <div class="foot">Sent by logwatch &bull; {{.Time}}</div>
</div>
</body>
</html>`))

type emailData struct {
	Icon        string
	HeaderColor string
	RuleName    string
	Severity    string
	Level       string
	Source      string
	Format      string
	Time        string
	Message     string
}

func renderEmailHTML(a rules.Alert) (string, error) {
	data := emailData{
		Icon:        severityIcon(a.Severity),
		HeaderColor: emailColour(a.Severity),
		RuleName:    a.RuleName,
		Severity:    a.Severity,
		Level:       a.Entry.Level,
		Source:      a.Entry.Source,
		Format:      string(a.Entry.Format),
		Time:        a.Time.Format(time.RFC1123),
		Message:     a.Entry.Message,
	}
	var buf bytes.Buffer
	if err := emailHTMLTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderEmailPlain(a rules.Alert) string {
	return fmt.Sprintf(
		"logwatch alert\n\nRule:     %s\nSeverity: %s\nLevel:    %s\nSource:   %s\nTime:     %s\n\nMessage:\n%s\n",
		a.RuleName, a.Severity, a.Entry.Level,
		a.Entry.Source, a.Time.Format(time.RFC1123),
		a.Entry.Message,
	)
}

func emailColour(severity string) string {
	switch severity {
	case "CRITICAL":
		return "#d93025"
	case "WARN":
		return "#f09d00"
	default:
		return "#1a73e8"
	}
}
