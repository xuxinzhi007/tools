package monitor

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxSummaryGroups = 1000

type SQLSummaryGroup struct {
	Fingerprint string
	Source      string
	Count       uint64
	Errors      uint64
	Slow        uint64
	MaxMS       float64
	HasDuration bool
}

type SQLSummary struct {
	// Kind distinguishes the report producer: "" (interval overview),
	// "threshold" (urgent alert) or "daily" (end-of-day digest).
	Kind            string
	Start           time.Time
	End             time.Time
	Total           uint64
	Errors          uint64
	Slow            uint64
	MissingDuration uint64
	Overflow        uint64
	Groups          []SQLSummaryGroup
}

// SummaryCollector aggregates error and slow-query counts per fingerprint over
// a rolling window. Drain resets the window.
type SummaryCollector struct {
	mu      sync.Mutex
	slowMS  float64
	summary SQLSummary
	groups  map[[32]byte]*SQLSummaryGroup
}

func NewSummaryCollector(slowMS float64, start time.Time) *SummaryCollector {
	return &SummaryCollector{slowMS: slowMS, summary: SQLSummary{Start: start}, groups: make(map[[32]byte]*SQLSummaryGroup)}
}

func (c *SummaryCollector) Record(e *Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.summary.Total++
	isError := e.Kind == "error" || e.Error != ""
	isSlow := e.ElapsedMS != nil && *e.ElapsedMS >= c.slowMS
	if e.ElapsedMS == nil {
		c.summary.MissingDuration++
	}
	if isError {
		c.summary.Errors++
	}
	if isSlow {
		c.summary.Slow++
	}
	if !isError && !isSlow {
		return
	}
	key := sha256.Sum256([]byte(e.Fingerprint + "\x00" + e.Source))
	group := c.groups[key]
	if group == nil {
		if len(c.groups) >= maxSummaryGroups {
			c.summary.Overflow++
			return
		}
		source, _ := limitText(e.Source, 240)
		group = &SQLSummaryGroup{Fingerprint: e.Fingerprint, Source: strings.Clone(source)}
		c.groups[key] = group
	}
	group.Count++
	if isError {
		group.Errors++
	}
	if isSlow {
		group.Slow++
	}
	if e.ElapsedMS != nil {
		group.HasDuration = true
		if *e.ElapsedMS > group.MaxMS {
			group.MaxMS = *e.ElapsedMS
		}
	}
}

func (c *SummaryCollector) Drain(end time.Time) SQLSummary {
	c.mu.Lock()
	result := c.summary
	result.End = end
	groups := c.groups
	c.summary = SQLSummary{Start: end}
	c.groups = make(map[[32]byte]*SQLSummaryGroup)
	c.mu.Unlock()
	for _, group := range groups {
		result.Groups = append(result.Groups, *group)
	}
	sort.Slice(result.Groups, func(i, j int) bool {
		a, b := result.Groups[i], result.Groups[j]
		if a.Errors != b.Errors {
			return a.Errors > b.Errors
		}
		if a.Slow != b.Slow {
			return a.Slow > b.Slow
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.MaxMS != b.MaxMS {
			return a.MaxMS > b.MaxMS
		}
		if a.Fingerprint != b.Fingerprint {
			return a.Fingerprint < b.Fingerprint
		}
		return a.Source < b.Source
	})
	if len(result.Groups) > 5 {
		result.Groups = result.Groups[:5]
	}
	return result
}

// RunSummaries drains the collector on every tick and forwards non-empty
// reports to output. It never closes output; the caller owns channel lifetime.
func RunSummaries(ctx context.Context, collector *SummaryCollector, interval time.Duration, output chan<- SQLSummary, diagnostics io.Writer) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report := collector.Drain(time.Now())
			if report.Errors == 0 && report.Slow == 0 {
				continue
			}
			select {
			case output <- report:
			default:
				fmt.Fprintln(diagnostics, "Feishu summary queue full; report dropped (SQL collection continues)")
			}
		}
	}
}

// RunThresholds drains the collector every window and emits an immediate alert
// when the window's error or slow-query count reaches a preset value. A fired
// alert suppresses further threshold alerts for cooldown. Each output channel
// receives the alert independently (typically Feishu plus an analyzer);
// lastFired advances once at least one channel accepts it.
func RunThresholds(ctx context.Context, collector *SummaryCollector, window, cooldown time.Duration, minErrors, minSlow uint64, diagnostics io.Writer, outputs ...chan<- SQLSummary) {
	ticker := time.NewTicker(window)
	defer ticker.Stop()
	lastFired := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report := collector.Drain(time.Now())
			triggered := (minErrors > 0 && report.Errors >= minErrors) || (minSlow > 0 && report.Slow >= minSlow)
			if !triggered {
				continue
			}
			if !lastFired.IsZero() && time.Since(lastFired) < cooldown {
				continue
			}
			report.Kind = "threshold"
			accepted := false
			for _, output := range outputs {
				select {
				case output <- report:
					accepted = true
				default:
					fmt.Fprintln(diagnostics, "threshold queue full; alert dropped for one channel (SQL collection continues)")
				}
			}
			if accepted {
				lastFired = time.Now()
			}
		}
	}
}

// nextDailyTime returns the next local occurrence of hour:minute strictly after
// now, wrapping to the following day when the time has already passed.
func nextDailyTime(hour, minute int, location *time.Location, now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, location)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// RunDaily drains the collector once per day at the local hour:minute and emits
// a digest when the day contained anomalies. The report covers everything since
// the previous report or process start.
func RunDaily(ctx context.Context, collector *SummaryCollector, hour, minute int, location *time.Location, output chan<- SQLSummary, diagnostics io.Writer) {
	if location == nil {
		location = time.Local
	}
	for {
		now := time.Now().In(location)
		timer := time.NewTimer(time.Until(nextDailyTime(hour, minute, location, now)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			report := collector.Drain(time.Now())
			if report.Errors == 0 && report.Slow == 0 {
				continue
			}
			report.Kind = "daily"
			select {
			case output <- report:
			default:
				fmt.Fprintln(diagnostics, "Feishu daily queue full; report dropped (SQL collection continues)")
			}
		}
	}
}
