package rules

import (
	"log"
	"regexp"
	"sync"
	"time"

	"github.com/sammug/logwatch/config"
)

type Alert struct {
	RuleName string
	Severity string
	Line     string
	Actions  []string
	Time     time.Time
}

type compiledRule struct {
	config.Rule
	regex     *regexp.Regexp
	lastFired time.Time
	mu        sync.Mutex
}

// Match reads lines, applies rules, and sends alerts.
func Match(ruleCfgs []config.Rule, lines <-chan string, alerts chan<- Alert) {
	compiled := make([]*compiledRule, 0, len(ruleCfgs))
	for _, r := range ruleCfgs {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			log.Printf("rules: invalid pattern [%s]: %v", r.Name, err)
			continue
		}
		compiled = append(compiled, &compiledRule{Rule: r, regex: re})
	}

	for line := range lines {
		for _, cr := range compiled {
			if cr.regex.MatchString(line) {
				cr.mu.Lock()
				elapsed := time.Since(cr.lastFired).Seconds()
				if cr.Cooldown > 0 && elapsed < float64(cr.Cooldown) {
					cr.mu.Unlock()
					continue
				}
				cr.lastFired = time.Now()
				cr.mu.Unlock()

				alerts <- Alert{
					RuleName: cr.Name,
					Severity: cr.Severity,
					Line:     line,
					Actions:  cr.Action,
					Time:     time.Now(),
				}
			}
		}
	}
}
