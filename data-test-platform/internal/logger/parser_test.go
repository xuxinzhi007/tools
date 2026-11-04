package logger

import (
	"testing"
	"time"
)

func TestParseJSONLog(t *testing.T) {
	parser := NewLogParser(200 * time.Millisecond)

	// 1. Normal SQL query
	line1 := `{"level":"DEBUG","ts":"2026-09-24T10:00:00+08:00","caller":"data/job.go:120","source":"internal/data/job.go:120","msg":"SQL Query","elapsed":"15.5ms","rows":5,"sql":"SELECT * FROM jobs WHERE status = 1","trace.id":"abc123trace","tenant_id":1,"uid":1001}`
	entry1 := parser.ParseLine(line1)
	if entry1 == nil {
		t.Fatalf("expected entry1 not nil")
	}
	if !entry1.IsSQL {
		t.Errorf("expected IsSQL true")
	}
	if entry1.IsSlowSQL {
		t.Errorf("expected IsSlowSQL false for 15.5ms")
	}
	if entry1.DurationMs != 15.5 {
		t.Errorf("expected 15.5ms, got %v", entry1.DurationMs)
	}
	if entry1.TraceID != "abc123trace" {
		t.Errorf("expected trace id 'abc123trace', got %s", entry1.TraceID)
	}

	// 2. Slow SQL query (> 200ms)
	line2 := `{"level":"WARN","ts":"2026-09-24T10:00:01+08:00","caller":"data/job.go:150","source":"internal/data/job.go:150","msg":"Slow SQL","elapsed":"320.45ms","threshold":"200ms","rows":100,"sql":"SELECT count(*) FROM episodes GROUP BY job_id","trace.id":"slowtrace999"}`
	entry2 := parser.ParseLine(line2)
	if entry2 == nil {
		t.Fatalf("expected entry2 not nil")
	}
	if !entry2.IsSQL || !entry2.IsSlowSQL {
		t.Errorf("expected Slow SQL, got is_sql=%v, is_slow=%v", entry2.IsSQL, entry2.IsSlowSQL)
	}
	if entry2.DurationMs != 320.45 {
		t.Errorf("expected 320.45ms, got %v", entry2.DurationMs)
	}

	// 3. Compact Logger line
	line3 := `[2026-09-24T10:00:02+08:00] DEBUG [data-service/inst-1/v1.0] [trace:compact123 span:span456 uid:1001 tenant:1] internal/data/task.go:88 - SQL Query | 12.3ms | 1 rows | SELECT * FROM tasks WHERE id = 1`
	entry3 := parser.ParseLine(line3)
	if entry3 == nil {
		t.Fatalf("expected entry3 not nil")
	}
	if !entry3.IsSQL {
		t.Errorf("expected entry3 IsSQL true")
	}
	if entry3.TraceID != "compact123" {
		t.Errorf("expected trace id 'compact123', got '%s'", entry3.TraceID)
	}
	if entry3.Source != "internal/data/task.go:88" {
		t.Errorf("expected source 'internal/data/task.go:88', got '%s'", entry3.Source)
	}
}
