package monitor

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestAggregateCumulativeAcrossEviction(t *testing.T) {
	m, err := New(filepath.Join(t.TempDir(), "unused.log"), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	store := NewAggregateStore(200)
	m.SetAggregateStore(store)
	for i := 0; i < 1000; i++ {
		m.Consume([]byte(fmt.Sprintf("{\"msg\":\"SQL Query\",\"sql\":\"SELECT 'secret-%d'\",\"elapsed\":\"10ms\",\"source\":\"svc.go:1\"}\n", i)))
	}
	m.Consume([]byte("{\"msg\":\"SQL Error\",\"sql\":\"SELECT 'secret'\",\"elapsed\":\"300ms\",\"source\":\"svc.go:1\"}\n"))
	if len(m.Snapshot().Events) != 1 {
		t.Fatal("retention should have evicted events")
	}
	snap := m.AggregateSnapshot()
	if len(snap.Groups) != 1 || snap.Groups[0].Count != 1001 || snap.Groups[0].Errors != 1 || snap.Groups[0].Slow != 1 || snap.Groups[0].Timed != 1001 {
		t.Fatalf("cumulative stats wrong: %+v", snap.Groups)
	}
	if snap.Groups[0].MaxMS != 300 {
		t.Fatalf("latency bounds wrong: %+v", snap.Groups[0])
	}
	data, _ := json.Marshal(snap)
	if strings.Contains(string(data), "secret") {
		t.Fatal("raw literal leaked into aggregate")
	}
}

func TestAggregateGroupingAndBounds(t *testing.T) {
	store := NewAggregateStore(200)
	store.Record(&Event{Fingerprint: "a", Source: "x.go:1", SQL: "SELECT 1", ReceivedAt: "t1", ElapsedMS: ptrFloat(100)})
	store.Record(&Event{Fingerprint: "a", Source: "x.go:1", SQL: "SELECT 1", ReceivedAt: "t2", ElapsedMS: ptrFloat(150)})
	store.Record(&Event{Fingerprint: "a", Source: "y.go:2", SQL: "SELECT 1", ReceivedAt: "t3", Kind: "error"})
	groups := store.Snapshot()
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].Count != 2 || groups[0].Source != "x.go:1" || groups[0].LastSeen != "t2" {
		t.Fatalf("group 1 wrong: %+v", groups[0])
	}

	bounded := NewAggregateStore(200)
	for i := 0; i < maxAggregateGroups+5; i++ {
		bounded.Record(&Event{Fingerprint: fmt.Sprint(i), Source: "s", SQL: "SELECT 1", ElapsedMS: ptrFloat(1)})
	}
	if bounded.Overflow() != 5 {
		t.Fatalf("overflow = %d, want 5", bounded.Overflow())
	}
	if len(bounded.Snapshot()) != maxAggregateGroups {
		t.Fatal("aggregate store not bounded")
	}
}

func ptrFloat(v float64) *float64 { return &v }

func TestAggregateN1Detection(t *testing.T) {
	store := NewAggregateStore(200)
	// 6 executions of the same fingerprint within one trace -> N+1 flag.
	for i := 0; i < 6; i++ {
		store.Record(&Event{Fingerprint: "fp", Source: "x.go:1", SQL: "SELECT 1", TraceID: "t1", ElapsedMS: ptrFloat(10)})
	}
	// Same fingerprint under a different trace stays below the threshold.
	store.Record(&Event{Fingerprint: "fp", Source: "x.go:1", SQL: "SELECT 1", TraceID: "t2", ElapsedMS: ptrFloat(10)})
	groups := store.Snapshot()
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if !g.N1 || g.N1Max != 6 {
		t.Fatalf("N+1 not flagged: N1=%v N1Max=%d", g.N1, g.N1Max)
	}
	if g.Score <= 0 || g.Reason == "" {
		t.Fatalf("score/reason missing: %+v", g)
	}
}

func TestAggregateHotspotScoreRanking(t *testing.T) {
	store := NewAggregateStore(200)
	// High error count ranks above plain high latency.
	store.Record(&Event{Fingerprint: "err", Source: "a.go:1", SQL: "SELECT 1", Kind: "error"})
	store.Record(&Event{Fingerprint: "err", Source: "a.go:1", SQL: "SELECT 1", Kind: "error"})
	slowMs := 5000.0
	store.Record(&Event{Fingerprint: "slow", Source: "b.go:1", SQL: "SELECT 2", ElapsedMS: &slowMs})

	ranked := RankHotspots(store.Snapshot())
	if len(ranked) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(ranked))
	}
	if ranked[0].Fingerprint != "err" {
		t.Fatalf("error group should rank first: %+v", ranked[0])
	}
	if ranked[0].Reason == "" || !strings.Contains(ranked[0].Reason, "错误") {
		t.Fatalf("reason should mention errors: %q", ranked[0].Reason)
	}
}
