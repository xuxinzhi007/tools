package monitor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func fakeClock(base time.Time) *time.Time {
	return &base
}

func TestTimeSeriesBucketingAndTrim(t *testing.T) {
	base := time.Unix(1_700_000_100, 0) // aligned to a 60s bucket boundary
	clock := fakeClock(base)
	advance := func(seconds int64) { *clock = base.Add(time.Duration(seconds) * time.Second) }

	c := NewTimeSeriesCollectorWithClock(200, 60, 5, func() time.Time { return *clock })
	err := &Event{Kind: "error", Error: "x"}
	slowMs := 250.0
	slow := &Event{ElapsedMS: &slowMs}

	advance(0)
	c.Record(err)
	c.Record(err)
	advance(60)
	c.Record(slow)
	advance(120)
	c.Record(err)

	snap := c.Snapshot()
	if snap.BucketSeconds != 60 || snap.SlowMS != 200 {
		t.Fatalf("meta: %+v", snap)
	}
	if len(snap.Buckets) != 3 {
		t.Fatalf("bucket count %d", len(snap.Buckets))
	}
	b0, b1, b2 := snap.Buckets[0], snap.Buckets[1], snap.Buckets[2]
	if b0.Errors != 2 || b0.Total != 2 || b0.Slow != 0 {
		t.Fatalf("b0: %+v", b0)
	}
	if b1.Slow != 1 || b1.Timed != 1 || b1.MaxMS != 250 || b1.Errors != 0 {
		t.Fatalf("b1: %+v", b1)
	}
	if b2.Errors != 1 {
		t.Fatalf("b2: %+v", b2)
	}

	// Advance beyond the 5-bucket window; the oldest bucket must be trimmed.
	advance(5 * 60)
	c.Record(err)
	snap = c.Snapshot()
	if len(snap.Buckets) != 3 {
		t.Fatalf("unexpected bucket count: %d", len(snap.Buckets))
	}
	if snap.Buckets[0].Start != base.Unix()+60 {
		t.Fatalf("oldest bucket not trimmed: %+v", snap.Buckets[0])
	}
}

func TestTimeSeriesSpikeDetection(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	clock := fakeClock(base)
	c := NewTimeSeriesCollectorWithClock(200, 60, 20, func() time.Time { return *clock })
	err := &Event{Kind: "error"}

	// Steady baseline: 1 error per bucket for 9 buckets.
	for i := 0; i < 9; i++ {
		*clock = base.Add(time.Duration(i*60) * time.Second)
		c.Record(err)
	}
	// Spike: 20 errors in one bucket.
	*clock = base.Add(time.Duration(9*60) * time.Second)
	for i := 0; i < 20; i++ {
		c.Record(err)
	}

	snap := c.Snapshot()
	var spiked int
	for _, b := range snap.Buckets {
		if b.ErrSpike {
			spiked++
			if b.Errors != 20 {
				t.Fatalf("wrong bucket flagged: %+v", b)
			}
		}
	}
	if spiked != 1 {
		t.Fatalf("expected exactly one spike, got %d", spiked)
	}
}

func TestTimeSeriesNoLeakInJSON(t *testing.T) {
	c := NewTimeSeriesCollectorWithClock(200, 60, 5, func() time.Time { return time.Unix(1_700_000_000, 0) })
	ms := 300.0
	c.Record(&Event{SQL: "SELECT 'secret'", Kind: "error", Error: "private", ElapsedMS: &ms})
	data, _ := json.Marshal(c.Snapshot())
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "private") {
		t.Fatal("raw data leaked into timeline snapshot")
	}
}
