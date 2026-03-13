# logwatch 🔍

A lightweight, fast log parser and alerting engine written in Go.

Watch log files in real-time, match patterns using regex rules, and fire alerts to stdout, files, or webhooks — with cooldown support to avoid alert spam.

---

## Features

- 📂 **Multi-file watching** — tail multiple log files simultaneously
- 🔍 **Regex pattern matching** — flexible rule definitions
- ⏱️ **Cooldown support** — suppress duplicate alerts within a time window
- 📤 **Multiple outputs** — stdout, file, Slack/webhook
- ⚙️ **INI config** — simple, human-readable configuration
- ⚡ **Lightweight** — single binary, zero overhead

---

## Installation

### Prerequisites
- Go 1.21+

### Build from source

```bash
git clone https://github.com/sammug/logwatch.git
cd logwatch
go build -o logwatch ./cmd/logwatch
```

### Run

```bash
./logwatch config/example.ini
```

---

## Configuration

Create a `.ini` config file:

```ini
[watch]
files = /var/log/app.log, /var/log/nginx/error.log

[output]
alert_file = /var/log/logwatch-alerts.log
webhook_url = https://hooks.slack.com/services/YOUR/WEBHOOK/URL

[rule:error-detected]
pattern   = ERROR|FATAL|panic
severity  = CRITICAL
action    = stdout, file, webhook
cooldown  = 5

[rule:auth-failure]
pattern   = authentication failure|Access denied
severity  = WARN
action    = stdout, file
cooldown  = 10

[rule:disk-warning]
pattern   = No space left|disk full
severity  = CRITICAL
action    = stdout, file
cooldown  = 60
```

### Config Reference

#### `[watch]`
| Key | Description |
|-----|-------------|
| `files` | Comma-separated list of log files to watch |

#### `[output]`
| Key | Description |
|-----|-------------|
| `alert_file` | Path to write alerts to |
| `webhook_url` | Slack-compatible webhook URL |

#### `[rule:<name>]`
| Key | Values | Description |
|-----|--------|-------------|
| `pattern` | regex string | Pattern to match against log lines |
| `severity` | `INFO`, `WARN`, `CRITICAL` | Alert severity level |
| `action` | `stdout`, `file`, `webhook` | Comma-separated output targets |
| `cooldown` | integer (seconds) | Min time between repeat alerts for this rule |

---

## Project Structure

```
logwatch/
├── cmd/
│   └── logwatch/
│       └── main.go          # Entry point
├── config/
│   ├── config.go            # Config loader
│   └── example.ini          # Sample configuration
├── internal/
│   ├── watcher/
│   │   └── watcher.go       # File tail + fsnotify
│   ├── rules/
│   │   └── rules.go         # Regex matcher + cooldown
│   ├── dispatcher/
│   │   └── dispatcher.go    # Alert outputs (stdout/file/webhook)
│   └── parser/              # (coming soon) structured log parsing
├── testdata/                # Test log files
├── go.mod
└── go.sum
```

---

## Architecture

```
Log file write
     │
     ▼
File Watcher (fsnotify)
     │  raw line
     ▼
Rule Engine ──── regex rules + cooldown
     │  Alert
     ▼
Alert Dispatcher
     ├──▶ stdout
     ├──▶ alert.log
     └──▶ Slack webhook
```

---

## Roadmap

### ✅ Done
- [x] Real-time file watching
- [x] Regex rule matching with cooldown / deduplication
- [x] Structured log parsing — JSON, syslog, Apache/Nginx, plaintext
- [x] Log rotation handling (rename + copytruncate)
- [x] Daemon + CLI control tool (`start` / `stop` / `status` / `tail` / `reload`)
- [x] Alert outputs — stdout, file, Slack webhook, Microsoft Teams, email (SMTP)
- [x] Web dashboard — live alert feed, expandable cards, parsed fields, copy buttons
- [x] Unit tests — parser, rules engine, config

### 🔜 Planned
- [ ] **stdin / pipe support** — `adb logcat | logwatch watch --stdin` for IDE integration
- [ ] **IDE plugins** — IntelliJ / Android Studio panel, VS Code extension
- [ ] **Named pipe support** — watch `mkfifo` pipes like regular files
- [ ] **Log rotation** — hot-reload config on SIGHUP (reload without restart)
- [ ] **Anomaly detection** — alert on spike in error rate, not just pattern match
- [ ] **On-call schedules** — route alerts to different people by time of day
- [ ] **Mobile push notifications** — iOS / Android alert delivery
- [ ] **Multi-server** — aggregate logs from multiple machines into one dashboard
- [ ] **logwatch Cloud** — hosted SaaS version, no server required

---

## Example Output

```
[2026-03-13T11:45:12+03:00] [CRITICAL] [error-detected] ERROR Database connection failed
[2026-03-13T11:45:15+03:00] [WARN]     [auth-failure]   authentication failure for user root
```

---

## License

MIT © Sammug
