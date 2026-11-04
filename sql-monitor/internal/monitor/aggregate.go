package monitor

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const (
	maxAggregateGroups = 10000
	// n1Threshold is the minimum number of times one fingerprint must repeat
	// inside a single trace before it is flagged as a suspected N+1 pattern.
	n1Threshold = 5
	// maxTraceFingerprints bounds the transient per-trace repetition counters.
	// When exceeded the counters reset; groups already flagged keep their N+1
	// evidence, so only future detections are affected.
	maxTraceFingerprints = 65536
)

// SQLAggregate is the process-lifetime cumulative statistics for one SQL
// fingerprint plus code location. Unlike the retained event window, it never
// shrinks when events are evicted, so the dashboard can answer "what has been
// happening overall", not just inside the recent window.
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
	FirstSeen   string  `json:"first_seen"`
	LastSeen    string  `json:"last_seen"`
	N1          bool    `json:"n1"`
	N1Max       uint64  `json:"n1_max"`
	Score       int64   `json:"score"`
	Reason      string  `json:"reason"`
}

// AggregateStore keeps cumulative per-fingerprint statistics across the whole
// monitor lifetime. It is bounded and never resets.
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

// Record folds one parsed SQL event into its fingerprint group. Events beyond
// maxAggregateGroups are dropped and counted as overflow rather than allowed to
// grow the store without bound.
func (s *AggregateStore) Record(e *Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sha256.Sum256([]byte(e.Fingerprint + "\x00" + e.Source))
	group := s.groups[key]
	if group == nil {
		if len(s.groups) >= maxAggregateGroups {
			s.overflow++
			return
		}
		source, _ := limitText(e.Source, 240)
		group = &SQLAggregate{
			Fingerprint: e.Fingerprint,
			Source:      strings.Clone(source),
			SQL:         strings.Clone(e.SQL),
			FirstSeen:   e.ReceivedAt,
		}
		s.groups[key] = group
	}
	group.Count++
	if e.Kind == "error" || e.Error != "" {
		group.Errors++
	}
	if e.ElapsedMS != nil {
		ms := *e.ElapsedMS
		group.Timed++
		group.SumMS += ms
		if group.Timed == 1 || ms > group.MaxMS {
			group.MaxMS = ms
		}
		if ms >= s.slowMS {
			group.Slow++
		}
	}
	if e.ReceivedAt != "" {
		group.LastSeen = e.ReceivedAt
	}
	if e.TraceID != "" {
		s.trackTraceLocked(group, e)
	}
}

// trackTraceLocked records one more occurrence of group's fingerprint within
// event's trace and flags the group as a suspected N+1 pattern once the
// repetition count reaches the threshold. Counters are transient: when the
// bounded map fills it resets, but groups already flagged keep their N+1
// evidence. Callers must hold s.mu.
func (s *AggregateStore) trackTraceLocked(group *SQLAggregate, e *Event) {
	if s.traceFp == nil {
		s.traceFp = make(map[[32]byte]uint32)
	}
	key := sha256.Sum256([]byte(e.TraceID + "\x00" + e.Fingerprint))
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

// hotspotScore weights the anomaly dimensions of a group into a single ranking
// number: errors dominate, then suspected N+1 and slow queries, then latency.
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

// hotspotReason explains in one line why a group ranks as a hotspot.
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

// Snapshot returns the cumulative groups sorted by impact: total executions,
// then errors, then slow executions, then peak latency. It returns a fresh
// slice; the strings inside are immutable and safe to share.
func (s *AggregateStore) Snapshot() []SQLAggregate {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]SQLAggregate, 0, len(s.groups))
	for _, group := range s.groups {
		group.Score = hotspotScore(group)
		group.Reason = hotspotReason(group)
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

// Overflow reports how many events were dropped because the group budget was
// exhausted. It is cumulative and monotonic.
func (s *AggregateStore) Overflow() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overflow
}

// RankHotspots returns a copy of groups sorted by weighted hotspot score
// (descending), with count, errors and fingerprint as stable tiebreakers. It is
// the ranking the analysis prompt and the UI's "热点加权" sort rely on.
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
