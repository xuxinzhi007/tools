package logger

import (
	"sort"
	"sync"
)

// StatsCollector collects and computes SQL statistics
type StatsCollector struct {
	mu           sync.RWMutex
	totalQueries uint64
	slowQueries  uint64
	fastQueries  uint64
	errorQueries uint64
	totalTimeMs  float64
	maxLatencyMs float64
	slowestSQLs  []SlowQueryRecord
}

// NewStatsCollector creates a new stats collector
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		slowestSQLs: make([]SlowQueryRecord, 0, 30),
	}
}

// Record processes a log entry for stats
func (s *StatsCollector) Record(entry *LogEntry) {
	if !entry.IsSQL {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalQueries++
	s.totalTimeMs += entry.DurationMs

	if entry.DurationMs > s.maxLatencyMs {
		s.maxLatencyMs = entry.DurationMs
	}

	if entry.Error != "" {
		s.errorQueries++
	}

	if entry.IsSlowSQL {
		s.slowQueries++

		record := SlowQueryRecord{
			ID:         entry.ID,
			Timestamp:  entry.Timestamp,
			TimeStr:    entry.TimeStr,
			Source:     entry.Source,
			SQL:        entry.SQL,
			DurationMs: entry.DurationMs,
			Elapsed:    entry.Elapsed,
			Rows:       entry.Rows,
			TraceID:    entry.TraceID,
			LogSource:  entry.LogSource,
			PodName:    entry.PodName,
		}

		s.slowestSQLs = append(s.slowestSQLs, record)

		// Sort by duration descending
		sort.Slice(s.slowestSQLs, func(i, j int) bool {
			return s.slowestSQLs[i].DurationMs > s.slowestSQLs[j].DurationMs
		})

		// Keep top 30
		if len(s.slowestSQLs) > 30 {
			s.slowestSQLs = s.slowestSQLs[:30]
		}
	} else {
		s.fastQueries++
	}
}

// GetStats returns current snapshot of SQL stats
func (s *StatsCollector) GetStats() SQLStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	avg := 0.0
	if s.totalQueries > 0 {
		avg = s.totalTimeMs / float64(s.totalQueries)
	}

	slowList := make([]SlowQueryRecord, len(s.slowestSQLs))
	copy(slowList, s.slowestSQLs)

	return SQLStats{
		TotalQueries: s.totalQueries,
		SlowQueries:  s.slowQueries,
		FastQueries:  s.fastQueries,
		ErrorQueries: s.errorQueries,
		AvgLatencyMs: avg,
		MaxLatencyMs: s.maxLatencyMs,
		SlowestSQLs:  slowList,
	}
}

// Clear resets the stats
func (s *StatsCollector) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalQueries = 0
	s.slowQueries = 0
	s.fastQueries = 0
	s.errorQueries = 0
	s.totalTimeMs = 0
	s.maxLatencyMs = 0
	s.slowestSQLs = make([]SlowQueryRecord, 0, 30)
}
