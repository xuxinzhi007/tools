package logger

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const (
	maxAggregateGroups   = 10000
	n1Threshold          = 5
	maxTraceFingerprints = 65536
)

// SQLAggregate is the process-lifetime cumulative statistics for one SQL
// fingerprint plus code location. Unlike the retained hub buffer it never
// shrinks when entries are evicted.
type SQLAggregate struct {
	Fingerprint string  `json:"fingerprint"`
	Source      string  `json:"source"`
	SQL         string  `json:"sql"`
	Count       uint64  `json:"count"`
	Errors      uint64  `json:"errors"`
	Slow        uint64  `json:"slow"`
	Timed       uint64  `json:"timed"`
	SumMS       float64 `json:"sum_ms"`
	MaxMS       float64 `json:"max_ms"`
	AvgMS       float64 `json:"avg_ms"`
	FirstSeen   string  `json:"first_seen"`
	LastSeen    string  `json:"last_seen"`
	N1          bool    `json:"n1"`
	N1Max       uint64  `json:"n1_max"`
	Score       int64   `json:"score"`
	Reason      string  `json:"reason"`
}

// AggregateStore keeps cumulative per-fingerprint statistics across the whole
// process lifetime. It is bounded and never resets.
type AggregateStore struct {
	mu       sync.Mutex
	slowMS   float64
	groups   map[[32]byte]*SQLAggregate
	overflow uint64
	traceFp  map[[32]byte]uint32
}

func NewAggregateStore(slowMS float64) *AggregateStore {
	return &AggregateStore{slowMS: slowMS, groups: make(map[[32]byte]*SQLAggregate)}
}

func (s *AggregateStore) SetSlowThreshold(ms float64) {
	s.mu.Lock()
	s.slowMS = ms
	s.mu.Unlock()
}

// Record folds one parsed SQL entry into its fingerprint group. Entries beyond
// maxAggregateGroups are dropped and counted as overflow.
func (s *AggregateStore) Record(entry *LogEntry) {
	if entry == nil || !entry.IsSQL {
		return
	}
	template, fingerprint, _ := RedactSQL(entry.SQL)
	if fingerprint == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := sha256.Sum256([]byte(fingerprint + "\x00" + entry.Source))
	group := s.groups[key]
	if group == nil {
		if len(s.groups) >= maxAggregateGroups {
			s.overflow++
			return
		}
		source, _ := LimitText(entry.Source, 240)
		sqlText, _ := LimitText(template, 4096)
		group = &SQLAggregate{
			Fingerprint: fingerprint,
			Source:      strings.Clone(source),
			SQL:         strings.Clone(sqlText),
			FirstSeen:   entry.TimeStr,
		}
		s.groups[key] = group
	}
	group.Count++
	if entry.Error != "" {
		group.Errors++
	}
	if entry.DurationMs > 0 || entry.Elapsed != "" {
		ms := entry.DurationMs
		group.Timed++
		group.SumMS += ms
		if group.Timed == 1 || ms > group.MaxMS {
			group.MaxMS = ms
		}
		if ms >= s.slowMS {
			group.Slow++
		}
	}
	if entry.TimeStr != "" {
		group.LastSeen = entry.TimeStr
	}
	if entry.TraceID != "" {
		s.trackTraceLocked(group, entry, fingerprint)
	}
}

// trackTraceLocked records one more occurrence of fingerprint within entry's
// trace and flags the group as suspected N+1 once the threshold is reached.
func (s *AggregateStore) trackTraceLocked(group *SQLAggregate, entry *LogEntry, fingerprint string) {
	if s.traceFp == nil {
		s.traceFp = make(map[[32]byte]uint32)
	}
	key := sha256.Sum256([]byte(entry.TraceID + "\x00" + fingerprint))
	n := s.traceFp[key] + 1
	s.traceFp[key] = n
	if n >= n1Threshold {
		group.N1 = true
		if uint64(n) > group.N1Max {
			group.N1Max = uint64(n)
		}
	}
	if len(s.traceFp) > maxTraceFingerprints {
		s.traceFp = make(map[[32]byte]uint32)
	}
}

func hotspotScore(g *SQLAggregate) int64 {
	score := int64(g.Errors)*10 + int64(g.Slow)*2
	if g.N1 {
		score += 8
	}
	if g.Timed > 0 {
		lat := int64(g.MaxMS / 500)
		if lat > 20 {
			lat = 20
		}
		score += lat
	}
	return score
}

func hotspotReason(g *SQLAggregate) string {
	var parts []string
	if g.Errors > 0 {
		parts = append(parts, fmt.Sprintf("执行错误 %d 次", g.Errors))
	}
	if g.Slow > 0 {
		parts = append(parts, fmt.Sprintf("慢查询 %d 次", g.Slow))
	}
	if g.N1 {
		parts = append(parts, fmt.Sprintf("疑似 N+1（单个 trace 最多重复 %d 次）", g.N1Max))
	}
	if g.Timed > 0 {
		parts = append(parts, fmt.Sprintf("峰值耗时 %g ms", g.MaxMS))
	}
	if len(parts) == 0 {
		return "执行次数较多"
	}
	return strings.Join(parts, "；")
}

// Snapshot returns cumulative groups sorted by total executions, then errors,
// then slow executions, then peak latency.
func (s *AggregateStore) Snapshot() []SQLAggregate {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]SQLAggregate, 0, len(s.groups))
	for _, group := range s.groups {
		group.Score = hotspotScore(group)
		group.Reason = hotspotReason(group)
		if group.Timed > 0 {
			group.AvgMS = group.SumMS / float64(group.Timed)
		}
		result = append(result, *group)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.Errors != b.Errors {
			return a.Errors > b.Errors
		}
		if a.Slow != b.Slow {
			return a.Slow > b.Slow
		}
		if a.MaxMS != b.MaxMS {
			return a.MaxMS > b.MaxMS
		}
		return a.Fingerprint < b.Fingerprint
	})
	return result
}

func (s *AggregateStore) Overflow() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overflow
}

func (s *AggregateStore) Clear() {
	s.mu.Lock()
	s.groups = make(map[[32]byte]*SQLAggregate)
	s.traceFp = nil
	s.overflow = 0
	s.mu.Unlock()
}

// RankHotspots returns a copy of groups sorted by weighted hotspot score.
func RankHotspots(groups []SQLAggregate) []SQLAggregate {
	result := append([]SQLAggregate(nil), groups...)
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.Errors != b.Errors {
			return a.Errors > b.Errors
		}
		return a.Fingerprint < b.Fingerprint
	})
	return result
}
