// Package rules matches parsed log entries against user-defined regex rules
// and emits Alerts for downstream dispatch.
package rules

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sammug/logitcat/config"
	"github.com/sammug/logitcat/internal/parser"
)

// Alert is produced whenever a log entry matches a rule.
type Alert struct {
	RuleName string
	Severity string
	Entry    parser.LogEntry
	Actions  []string
	Time     time.Time
}

// compiled wraps a config.Rule with its pre-compiled regex and cooldown state.
type compiled struct {
	config.Rule
	re        *regexp.Regexp
	mu        sync.Mutex
	lastFired time.Time
}

// Match reads LogEntries, tests each against all rules, and sends Alerts.
func Match(ruleCfgs []config.Rule, in <-chan parser.LogEntry, out chan<- Alert) {
	rules := compile(ruleCfgs)

	for entry := range in {
		for _, r := range rules {
			if r.matches(entry) && r.cooldownOK() {
				out <- Alert{
					RuleName: r.Name,
					Severity: r.Severity,
					Entry:    entry,
					Actions:  r.Action,
					Time:     time.Now(),
				}
			}
		}
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// compile pre-compiles all rule patterns, skipping invalid ones.
func compile(cfgs []config.Rule) []*compiled {
	out := make([]*compiled, 0, len(cfgs))
	for _, cfg := range cfgs {
		re, err := regexp.Compile(cfg.Pattern)
		if err != nil {
			// Log via standard logger; don't crash the whole daemon.
			continue
		}
		out = append(out, &compiled{Rule: cfg, re: re})
	}
	return out
}

// matches returns true when the entry satisfies the rule's field + pattern + level filters.
func (c *compiled) matches(e parser.LogEntry) bool {
	// Optional level filter (e.g. only match entries at "error" or above).
	if c.Rule.Level != "" && !levelAtLeast(e.Level, c.Rule.Level) {
		return false
	}

	// Determine which string to test the pattern against.
	target := resolveField(e, c.Rule.Field)
	return c.re.MatchString(target)
}

// cooldownOK returns true and records the fire time if the cooldown has elapsed.
func (c *compiled) cooldownOK() bool {
	if c.Cooldown == 0 {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.lastFired) < time.Duration(c.Cooldown)*time.Second {
		return false
	}
	c.lastFired = time.Now()
	return true
}

// resolveField returns the value of the named field from the entry.
// An empty field name defaults to matching against the full message.
func resolveField(e parser.LogEntry, field string) string {
	switch strings.ToLower(field) {
	case "", "message", "msg":
		return e.Message
	case "raw":
		return e.Raw
	case "level":
		return e.Level
	case "source":
		return e.Source
	default:
		return e.Fields[field]
	}
}

// levelOrder defines severity from lowest (0) to highest.
var levelOrder = map[string]int{
	"debug": 0,
	"info":  1,
	"warn":  2,
	"error": 3,
	"fatal": 4,
}

// levelAtLeast returns true when actual is at least as severe as minimum.
func levelAtLeast(actual, minimum string) bool {
	a, aOK := levelOrder[strings.ToLower(actual)]
	m, mOK := levelOrder[strings.ToLower(minimum)]
	if !aOK || !mOK {
		return true // unknown levels pass through
	}
	return a >= m
}
