package logger

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var (
	ansiRegex     = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	compactRegex  = regexp.MustCompile(`^\[(.*?)\]\s+([A-Z]+)\s+\[(.*?)\]\s+\[(.*?)\]\s+(.*?)\s+-\s+(.*)$`)
	gormFastRegex = regexp.MustCompile(`^\[([0-9\.]+(?:ms|µs|s))\]\s+\[rows:([0-9]+)\]\s+(.*)$`)
)

// StripANSI removes terminal color codes
func StripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

// LogParser handles parsing various log formats into LogEntry
type LogParser struct {
	nextID        uint64
	slowThreshold time.Duration
}

// NewLogParser creates a new parser
func NewLogParser(slowThreshold time.Duration) *LogParser {
	if slowThreshold <= 0 {
		slowThreshold = 200 * time.Millisecond
	}
	return &LogParser{
		slowThreshold: slowThreshold,
	}
}

// SetSlowThreshold updates threshold
func (p *LogParser) SetSlowThreshold(d time.Duration) {
	if d > 0 {
		p.slowThreshold = d
	}
}

// ParseLine parses a single log line into a LogEntry
func (p *LogParser) ParseLine(rawLine string) *LogEntry {
	cleaned := strings.TrimSpace(StripANSI(rawLine))
	if cleaned == "" {
		return nil
	}

	entryID := atomic.AddUint64(&p.nextID, 1)
	entry := &LogEntry{
		ID:        entryID,
		Timestamp: time.Now(),
		TimeStr:   time.Now().Format("15:04:05.000"),
		Level:     "INFO",
		Raw:       rawLine,
	}

	// 1. Try parsing JSON format
	if strings.HasPrefix(cleaned, "{") && strings.HasSuffix(cleaned, "}") {
		if p.parseJSON(cleaned, entry) {
			return entry
		}
	}

	// 2. Try parsing Kratos Compact format
	if strings.HasPrefix(cleaned, "[") {
		if p.parseCompact(cleaned, entry) {
			return entry
		}
	}

	// 3. Fallback: check if line contains SQL statements
	p.parseRawFallback(cleaned, entry)
	return entry
}

func (p *LogParser) parseJSON(line string, entry *LogEntry) bool {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		return false
	}

	if lvl, ok := m["level"].(string); ok {
		entry.Level = strings.ToUpper(strings.TrimSpace(lvl))
	}

	if tsStr, ok := m["ts"].(string); ok {
		if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
			entry.Timestamp = t
			entry.TimeStr = t.Format("15:04:05.000")
		} else {
			entry.TimeStr = tsStr
		}
	}

	if msg, ok := m["msg"].(string); ok {
		entry.Message = msg
	}

	if src, ok := m["source"].(string); ok && src != "" {
		entry.Source = src
	} else if caller, ok := m["caller"].(string); ok {
		entry.Source = caller
	}

	// Kratos JSONLogger emits either flat "trace.id" or a nested
	// {"trace":{"id":"..","span_id":".."}} object depending on version and
	// whether the trace filter is active. Accept every shape.
	if traceID, ok := m["trace.id"].(string); ok && traceID != "" {
		entry.TraceID = traceID
	}
	if spanID, ok := m["span.id"].(string); ok && spanID != "" {
		entry.SpanID = spanID
	}
	if traceObj, ok := m["trace"].(map[string]interface{}); ok {
		if entry.TraceID == "" {
			if tid, ok := traceObj["id"].(string); ok {
				entry.TraceID = tid
			}
		}
		if entry.SpanID == "" {
			for _, key := range []string{"span_id", "spanId", "id"} {
				if sid, ok := traceObj[key].(string); ok && sid != "" {
					entry.SpanID = sid
					break
				}
			}
		}
	}
	if entry.TraceID == "" {
		for _, key := range []string{"trace_id", "traceId"} {
			if tid, ok := m[key].(string); ok && tid != "" {
				entry.TraceID = tid
				break
			}
		}
	}
	if entry.SpanID == "" {
		for _, key := range []string{"span_id", "spanId"} {
			if sid, ok := m[key].(string); ok && sid != "" {
				entry.SpanID = sid
				break
			}
		}
	}
	if uid, ok := m["uid"]; ok && uid != nil {
		entry.UID = fmt.Sprint(uid)
	}
	if tid, ok := m["tenant_id"]; ok && tid != nil {
		entry.TenantID = fmt.Sprint(tid)
	}
	if errMsg, ok := m["error"].(string); ok {
		entry.Error = errMsg
	}

	// Check if this is a SQL log entry
	sqlVal, hasSQL := m["sql"].(string)
	isSQLMsg := entry.Message == "SQL Query" || entry.Message == "Slow SQL" || entry.Message == "SQL Error"

	if hasSQL || isSQLMsg {
		entry.IsSQL = true
		entry.SQL = sqlVal

		if elapsedStr, ok := m["elapsed"].(string); ok {
			entry.Elapsed = elapsedStr
			entry.DurationMs = parseDurationMs(elapsedStr)
		}

		if rowsVal, ok := m["rows"]; ok {
			switch v := rowsVal.(type) {
			case float64:
				entry.Rows = int64(v)
			case int64:
				entry.Rows = v
			case int:
				entry.Rows = int64(v)
			}
		}

		if thresholdStr, ok := m["threshold"].(string); ok {
			entry.Threshold = thresholdStr
		}

		// Check if slow SQL
		thresholdMs := float64(p.slowThreshold.Microseconds()) / 1000.0
		if entry.Message == "Slow SQL" || entry.DurationMs >= thresholdMs {
			entry.IsSlowSQL = true
		}
	}

	return true
}

func (p *LogParser) parseCompact(line string, entry *LogEntry) bool {
	matches := compactRegex.FindStringSubmatch(line)
	if len(matches) < 7 {
		return false
	}

	tsStr := matches[1]
	levelStr := strings.TrimSpace(matches[2])
	tracePart := matches[4]
	callerStr := matches[5]
	msgAndContext := matches[6]

	if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
		entry.Timestamp = t
		entry.TimeStr = t.Format("15:04:05.000")
	} else {
		entry.TimeStr = tsStr
	}

	entry.Level = strings.ToUpper(levelStr)
	entry.Source = callerStr

	// Extract trace & span from tracePart
	for _, segment := range strings.Split(tracePart, " ") {
		if strings.HasPrefix(segment, "trace:") {
			entry.TraceID = strings.TrimPrefix(segment, "trace:")
		} else if strings.HasPrefix(segment, "span:") {
			entry.SpanID = strings.TrimPrefix(segment, "span:")
		} else if strings.HasPrefix(segment, "uid:") {
			entry.UID = strings.TrimPrefix(segment, "uid:")
		} else if strings.HasPrefix(segment, "tenant:") {
			entry.TenantID = strings.TrimPrefix(segment, "tenant:")
		}
	}

	// Check if message has pipes "msg | context..." or GORM logger format
	parts := strings.Split(msgAndContext, " | ")
	entry.Message = strings.TrimSpace(parts[0])

	isSQL := strings.Contains(msgAndContext, "SQL Query") ||
		strings.Contains(msgAndContext, "Slow SQL") ||
		strings.Contains(msgAndContext, "SQL Error") ||
		strings.Contains(msgAndContext, "sql=") ||
		strings.Contains(msgAndContext, "SELECT ") ||
		strings.Contains(msgAndContext, "INSERT ") ||
		strings.Contains(msgAndContext, "UPDATE ") ||
		strings.Contains(msgAndContext, "DELETE ")

	if isSQL {
		entry.IsSQL = true
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "source=") {
				entry.Source = strings.TrimPrefix(part, "source=")
			} else if part == "SQL Query" || part == "Slow SQL" || part == "SQL Error" {
				entry.Message = part
			} else if strings.HasPrefix(part, "elapsed=") {
				durStr := strings.TrimPrefix(part, "elapsed=")
				entry.Elapsed = durStr
				entry.DurationMs = parseDurationMs(durStr)
			} else if strings.HasPrefix(part, "rows=") {
				rStr := strings.TrimPrefix(part, "rows=")
				entry.Rows, _ = strconv.ParseInt(rStr, 10, 64)
			} else if strings.HasPrefix(part, "sql=") {
				entry.SQL = strings.TrimPrefix(part, "sql=")
			} else if strings.HasSuffix(part, "rows") {
				fields := strings.Fields(part)
				if len(fields) > 0 {
					entry.Rows, _ = strconv.ParseInt(fields[0], 10, 64)
				}
			} else if strings.HasPrefix(part, "threshold:") {
				entry.Threshold = strings.TrimPrefix(part, "threshold:")
			} else if strings.HasPrefix(part, "error:") {
				entry.Error = strings.TrimPrefix(part, "error:")
			} else if strings.HasSuffix(part, "ms") || strings.HasSuffix(part, "s") || strings.HasSuffix(part, "µs") {
				entry.Elapsed = part
				entry.DurationMs = parseDurationMs(part)
			} else if isSQLQueryString(part) {
				entry.SQL = part
			}
		}

		thresholdMs := float64(p.slowThreshold.Microseconds()) / 1000.0
		if entry.Message == "Slow SQL" || entry.DurationMs >= thresholdMs {
			entry.IsSlowSQL = true
		}
	}

	return true
}

func (p *LogParser) parseRawFallback(line string, entry *LogEntry) {
	entry.Message = line

	// Detect log level keywords
	upper := strings.ToUpper(line)
	switch {
	case strings.Contains(upper, "ERROR"):
		entry.Level = "ERROR"
	case strings.Contains(upper, "WARN"):
		entry.Level = "WARN"
	case strings.Contains(upper, "DEBUG"):
		entry.Level = "DEBUG"
	default:
		entry.Level = "INFO"
	}

	// Detect SQL query
	if isSQLQueryString(line) {
		entry.IsSQL = true
		entry.SQL = line

		// Try gormFastRegex e.g. [12.5ms] [rows:1] SELECT ...
		if m := gormFastRegex.FindStringSubmatch(line); len(m) >= 4 {
			entry.Elapsed = m[1]
			entry.DurationMs = parseDurationMs(m[1])
			entry.Rows, _ = strconv.ParseInt(m[2], 10, 64)
			entry.SQL = m[3]
		}

		thresholdMs := float64(p.slowThreshold.Microseconds()) / 1000.0
		if strings.Contains(upper, "SLOW SQL") || entry.DurationMs >= thresholdMs {
			entry.IsSlowSQL = true
		}
	}
}

// parseDurationMs converts strings like "24.5ms", "1.2s", "800µs" to milliseconds float64
func parseDurationMs(durStr string) float64 {
	d, err := time.ParseDuration(durStr)
	if err == nil {
		return float64(d.Microseconds()) / 1000.0
	}
	// Try float parse directly
	numStr := strings.TrimRight(durStr, "msµsun ")
	val, err := strconv.ParseFloat(numStr, 64)
	if err == nil {
		return val
	}
	return 0
}

func isSQLQueryString(str string) bool {
	upper := strings.ToUpper(strings.TrimSpace(str))
	return strings.HasPrefix(upper, "SELECT ") ||
		strings.HasPrefix(upper, "INSERT ") ||
		strings.HasPrefix(upper, "UPDATE ") ||
		strings.HasPrefix(upper, "DELETE ") ||
		strings.Contains(upper, " FROM ") ||
		strings.Contains(upper, " INTO ") ||
		strings.Contains(upper, " WHERE ")
}
