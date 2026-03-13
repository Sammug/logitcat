package config

import (
	"strings"

	"gopkg.in/ini.v1"
)

type Rule struct {
	Name     string
	Pattern  string
	Severity string
	Action   []string
	Cooldown int // seconds
}

type Config struct {
	Files      []string
	Rules      []Rule
	WebhookURL string
	AlertFile  string
}

func Load(path string) (*Config, error) {
	cfg, err := ini.Load(path)
	if err != nil {
		return nil, err
	}

	c := &Config{}

	// [watch] section
	if sec, err := cfg.GetSection("watch"); err == nil {
		files := sec.Key("files").String()
		for _, f := range strings.Split(files, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				c.Files = append(c.Files, f)
			}
		}
	}

	// [output] section
	if sec, err := cfg.GetSection("output"); err == nil {
		c.WebhookURL = sec.Key("webhook_url").String()
		c.AlertFile = sec.Key("alert_file").String()
	}

	// [rule:*] sections
	for _, sec := range cfg.Sections() {
		if !strings.HasPrefix(sec.Name(), "rule:") {
			continue
		}
		name := strings.TrimPrefix(sec.Name(), "rule:")
		actions := strings.Split(sec.Key("action").String(), ",")
		for i := range actions {
			actions[i] = strings.TrimSpace(actions[i])
		}
		rule := Rule{
			Name:     name,
			Pattern:  sec.Key("pattern").String(),
			Severity: sec.Key("severity").MustString("INFO"),
			Action:   actions,
			Cooldown: sec.Key("cooldown").MustInt(0),
		}
		c.Rules = append(c.Rules, rule)
	}

	return c, nil
}
