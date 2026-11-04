package logger

import "time"

// LogEntry represents a structured log entry for the UI
type LogEntry struct {
	ID        uint64    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	TimeStr   string    `json:"time_str"`
	Level     string    `json:"level"` // DEBUG, INFO, WARN, ERROR, FATAL
	Message   string    `json:"message"`
	Source    string    `json:"source,omitempty"` // Caller code location e.g. internal/data/job.go:120

	// Tracing & Context
	TraceID  string `json:"trace_id,omitempty"`
	SpanID   string `json:"span_id,omitempty"`
	TenantID string `json:"tenant_id,omitempty"`
	UID      string `json:"uid,omitempty"`

	// SQL Monitoring Fields
	IsSQL      bool    `json:"is_sql"`
	IsSlowSQL  bool    `json:"is_slow_sql"`
	SQL        string  `json:"sql,omitempty"`
	DurationMs float64 `json:"duration_ms,omitempty"`
	Elapsed    string  `json:"elapsed,omitempty"`
	Rows       int64   `json:"rows,omitempty"`
	Threshold  string  `json:"threshold,omitempty"`
	Error      string  `json:"error,omitempty"`

	// Cloud / Container Info
	PodName       string `json:"pod_name,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	ProjectName   string `json:"project_name,omitempty"`
	LogSource     string `json:"log_source,omitempty"` // "local", "dev", "test"

	Raw string `json:"raw"`
}

// SlowQueryRecord records detail of a slow SQL query
type SlowQueryRecord struct {
	ID         uint64    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	TimeStr    string    `json:"time_str"`
	Source     string    `json:"source"`
	SQL        string    `json:"sql"`
	DurationMs float64   `json:"duration_ms"`
	Elapsed    string    `json:"elapsed"`
	Rows       int64     `json:"rows"`
	TraceID    string    `json:"trace_id,omitempty"`
	LogSource  string    `json:"log_source,omitempty"` // local / dev / test / prod
	PodName    string    `json:"pod_name,omitempty"`
}

// SQLStats represents aggregated statistics for SQL queries
type SQLStats struct {
	TotalQueries uint64            `json:"total_queries"`
	SlowQueries  uint64            `json:"slow_queries"`
	FastQueries  uint64            `json:"fast_queries"`
	ErrorQueries uint64            `json:"error_queries"`
	AvgLatencyMs float64           `json:"avg_latency_ms"`
	MaxLatencyMs float64           `json:"max_latency_ms"`
	SlowestSQLs  []SlowQueryRecord `json:"slowest_sqls"`
}
