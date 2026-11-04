package config

import (
	"errors"
	"math"
	"net/mail"
	"strings"
)

type FeishuRecipient struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type FeishuConfig struct {
	Enabled         bool              `json:"enabled"`
	AppID           string            `json:"app_id"`
	AppSecret       string            `json:"app_secret"`
	Recipients      []FeishuRecipient `json:"recipients"`
	IntervalSeconds int               `json:"interval_seconds"`
	SlowMS          float64           `json:"slow_ms"`
	Project         string            `json:"project"`
	Threshold       *FeishuThreshold  `json:"threshold,omitempty"`
	Daily           *FeishuDaily      `json:"daily,omitempty"`
}

// FeishuThreshold triggers an immediate alert when a short window's error or
// slow-query count reaches a preset value. A zero errors/slow value disables
// that dimension; at least one must be positive.
type FeishuThreshold struct {
	Enabled         bool   `json:"enabled"`
	WindowSeconds   int    `json:"window_seconds"`
	Errors          uint64 `json:"errors"`
	Slow            uint64 `json:"slow"`
	CooldownSeconds int    `json:"cooldown_seconds"`
}

// FeishuDaily sends a once-per-day digest of the anomalies collected since the
// previous report (or process start) at the local hour:minute.
type FeishuDaily struct {
	Enabled bool `json:"enabled"`
	Hour    int  `json:"hour"`
	Minute  int  `json:"minute"`
}

func (c *FeishuConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	c.AppID = strings.TrimSpace(c.AppID)
	c.AppSecret = strings.TrimSpace(c.AppSecret)
	c.Project = strings.TrimSpace(c.Project)
	if c.AppID == "" || c.AppSecret == "" {
		return errors.New("feishu requires app_id and app_secret")
	}
	if c.IntervalSeconds < 10 || c.IntervalSeconds > 86400 {
		return errors.New("feishu interval_seconds must be 10..86400")
	}
	if math.IsNaN(c.SlowMS) || math.IsInf(c.SlowMS, 0) || c.SlowMS <= 0 {
		return errors.New("feishu slow_ms must be positive and finite")
	}
	if c.Project == "" || len(c.Project) > 120 {
		return errors.New("feishu project must be 1..120 bytes")
	}
	if len(c.Recipients) == 0 || len(c.Recipients) > 20 {
		return errors.New("feishu requires 1..20 recipients")
	}
	seen := make(map[string]bool)
	for i := range c.Recipients {
		r := &c.Recipients[i]
		r.Email = strings.TrimSpace(r.Email)
		address, err := mail.ParseAddress(r.Email)
		if err != nil || address.Address != r.Email || len(r.Email) > 254 || len(r.Name) > 120 {
			return errors.New("feishu recipient requires a plain email address and name of at most 120 bytes")
		}
		key := strings.ToLower(r.Email)
		if seen[key] {
			return errors.New("feishu recipients must have unique emails")
		}
		seen[key] = true
	}
	if err := c.Threshold.validate(); err != nil {
		return err
	}
	if err := c.Daily.validate(); err != nil {
		return err
	}
	return nil
}

func (t *FeishuThreshold) validate() error {
	if t == nil || !t.Enabled {
		return nil
	}
	if t.WindowSeconds == 0 {
		t.WindowSeconds = 60
	}
	if t.WindowSeconds < 5 || t.WindowSeconds > 86400 {
		return errors.New("feishu threshold window_seconds must be 5..86400")
	}
	if t.Errors == 0 && t.Slow == 0 {
		return errors.New("feishu threshold requires errors or slow greater than zero")
	}
	if t.CooldownSeconds == 0 {
		t.CooldownSeconds = 900
	}
	if t.CooldownSeconds < 1 || t.CooldownSeconds > 604800 {
		return errors.New("feishu threshold cooldown_seconds must be 1..604800")
	}
	return nil
}

func (d *FeishuDaily) validate() error {
	if d == nil || !d.Enabled {
		return nil
	}
	if d.Hour < 0 || d.Hour > 23 || d.Minute < 0 || d.Minute > 59 {
		return errors.New("feishu daily hour must be 0..23 and minute 0..59")
	}
	return nil
}
