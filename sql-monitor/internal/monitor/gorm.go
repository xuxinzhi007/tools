package monitor

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	gormHeader  = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) (.+?\.go:[0-9]+)(?: (.*))?$`)
	gormMetrics = regexp.MustCompile(`^\[([0-9]+(?:\.[0-9]+)?ms)\] \[rows:(-|[0-9]+)\] (.+)$`)
	gormSlow    = regexp.MustCompile(`^SLOW SQL >= (.+)$`)
)

// A native GORM entry has one header followed immediately by one SQL line.
// Continuations are never guessed: physical SQL lines are marked incomplete.
func (m *Monitor) parsePhysicalLine(line []byte) {
	s := strings.TrimRight(ansiPattern.ReplaceAllString(string(line), ""), "\r\n")
	if !utf8.Valid(line) {
		m.resetGORM()
		m.status.ParseErrors++
		return
	}
	if header := gormHeader.FindStringSubmatch(s); header != nil {
		m.resetGORM()
		stamp, err := time.ParseInLocation("2006/01/02 15:04:05", header[1], time.Local)
		if err != nil || len(s) > maxError+1024 {
			m.status.ParseErrors++
			return
		}
		record := map[string]any{"ts": stamp.Format(time.RFC3339), "source": header[2], "msg": "SQL Query", "level": "INFO", "truncated": true}
		if header[3] != "" {
			slow := gormSlow.FindStringSubmatch(header[3])
			if slow != nil && durationMS(slow[1]) != nil {
				record["msg"], record["level"] = "Slow SQL", "WARN"
			} else {
				record["msg"], record["level"], record["error"] = "SQL Error", "ERROR", header[3]
			}
		}
		m.gormPending = record
		return
	}
	if metrics := gormMetrics.FindStringSubmatch(s); metrics != nil {
		pending := m.gormPending
		m.gormPending = nil
		if pending == nil || !sqlStart(metrics[3]) || durationMS(metrics[1]) == nil || metrics[2] != "-" && rowsValue(metrics[2]) == nil {
			m.status.ParseErrors++
			return
		}
		pending["elapsed"], pending["rows"], pending["sql"] = metrics[1], metrics[2], metrics[3]
		event, raw, err := recordEvent(pending)
		m.accept(event, raw, err, "gorm")
		return
	}
	if m.gormPending != nil && (strings.HasPrefix(s, "[info] ") || strings.HasPrefix(s, "[warn] ") || strings.HasPrefix(s, "[error] ")) {
		m.gormPending = nil
		m.status.IgnoredLines += 2
		return
	}
	m.resetGORM()
	if strings.HasPrefix(s, "[") && strings.Contains(s, "] [rows:") {
		m.status.ParseErrors++
		return
	}
	event, raw, err := parseLine(line)
	format := "compact"
	if strings.HasPrefix(strings.TrimSpace(s), "{") {
		format = "json"
	}
	m.accept(event, raw, err, format)
}

func (m *Monitor) resetGORM() {
	if m.gormPending != nil {
		m.status.IncompleteRecords++
		m.gormPending = nil
	}
}

func (m *Monitor) accept(event *Event, raw RawEvent, err error, format string) {
	switch {
	case err != nil:
		m.status.ParseErrors++
	case event == nil:
		m.status.IgnoredLines++
	default:
		if len(m.summaries) > 0 {
			for _, s := range m.summaries {
				s.Record(event)
			}
		}
		if m.aggregates != nil {
			m.aggregates.Record(event)
		}
		if m.timeline != nil {
			m.timeline.Record(event)
		}
		m.status.SQLObserved++
		if event.ElapsedMS != nil {
			m.status.WithDuration++
		}
		if event.Source != "" {
			m.status.WithSource++
		}
		if event.TraceID != "" {
			m.status.WithTrace++
		}
		switch format {
		case "json":
			m.status.Formats.JSON++
		case "compact":
			m.status.Formats.Compact++
		case "gorm":
			m.status.Formats.GORM++
		}
		event.ID = m.nextID
		m.nextID++
		m.events = append(m.events, *event)
		m.raw[event.ID] = raw
		m.rawBytes += rawSize(event, raw)
		for m.eventCount() > m.status.Capacity || (m.rawBudget > 0 && m.rawBytes > m.rawBudget) {
			m.evictOldest()
		}
	}
}

// rawSize estimates the retained bytes for an event: the masked template, code
// location, error placeholder, raw SQL and raw error, plus fixed overhead.
func rawSize(event *Event, raw RawEvent) int64 {
	const overhead = 256
	return int64(len(event.SQL)+len(event.Source)+len(event.Error)+len(raw.SQL)+len(raw.Error)) + overhead
}

// evictOldest drops the oldest retained event. Callers must hold m.mu. The
// events slice is a deque with a head index; the tail is compacted in place
// only when at least half the slice is evicted, keeping eviction O(1).
func (m *Monitor) evictOldest() {
	ev := m.events[m.eventHead]
	m.rawBytes -= rawSize(&ev, m.raw[ev.ID])
	delete(m.raw, ev.ID)
	m.eventHead++
	m.status.DroppedEvents++
	if m.eventHead == len(m.events) {
		m.events = m.events[:0]
		m.eventHead = 0
	} else if m.eventHead > 1024 && m.eventHead*2 >= len(m.events) {
		m.events = m.events[m.eventHead:]
		m.eventHead = 0
	}
}
