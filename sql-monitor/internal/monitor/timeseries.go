package monitor

import (
	"sort"
	"sync"
	"time"
)

const (
	defaultTimeBucketSeconds = 60
	// defaultTimeWindow is the number of buckets retained (60s × 1440 = 24h).
	defaultTimeWindow = 1440
)

// TimeBucket is one fixed-width slice of the anomaly timeline. Counts are
// cumulative within the bucket; spike flags are computed against the whole
// window's median at snapshot time.
type TimeBucket struct {
	Start     int64   `json:"start"` // unix seconds, bucket-aligned
	Total     uint64  `json:"total"`
	Errors    uint64  `json:"errors"`
	Slow      uint64  `json:"slow"`
	Timed     uint64  `json:"timed"`
	SumMS     float64 `json:"sum_ms"`
	MaxMS     float64 `json:"max_ms"`
	ErrSpike  bool    `json:"err_spike"`
	SlowSpike bool    `json:"slow_spike"`
}

// TimeSeriesSnapshot is the payload served to the UI for the anomaly timeline.
type TimeSeriesSnapshot struct {
	BucketSeconds int64        `json:"bucket_seconds"`
	SlowMS        float64      `json:"slow_ms"`
	Buckets       []TimeBucket `json:"buckets"`
}

// TimeSeriesCollector buckets error and slow-query counts over time so the UI
// can draw an anomaly timeline and flag rate spikes. It keeps a rolling window
// and never resets.
type TimeSeriesCollector struct {
	mu      sync.Mutex
	slowMS  float64
	bucket  int64
	window  int
	now     func() time.Time
	buckets []TimeBucket
}

// NewTimeSeriesCollector returns a collector with the default bucket width
// (60s) and window (24h).
func NewTimeSeriesCollector(slowMS float64) *TimeSeriesCollector {
	return NewTimeSeriesCollectorWithClock(slowMS, defaultTimeBucketSeconds, defaultTimeWindow, time.Now)
}

func NewTimeSeriesCollectorWithClock(slowMS float64, bucketSeconds int64, window int, clock func() time.Time) *TimeSeriesCollector {
	if bucketSeconds < 1 {
		bucketSeconds = defaultTimeBucketSeconds
	}
	if window < 1 {
		window = defaultTimeWindow
	}
	return &TimeSeriesCollector{slowMS: slowMS, bucket: bucketSeconds, window: window, now: clock}
}

func (c *TimeSeriesCollector) Record(e *Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.now().Unix() / c.bucket
	start := idx * c.bucket
	n := len(c.buckets)
	if n == 0 || c.buckets[n-1].Start != start {
		c.buckets = append(c.buckets, TimeBucket{Start: start})
		n++
	}
	b := &c.buckets[n-1]
	b.Total++
	if e.Kind == "error" || e.Error != "" {
		b.Errors++
	}
	if e.ElapsedMS != nil {
		ms := *e.ElapsedMS
		b.Timed++
		b.SumMS += ms
		if b.Timed == 1 || ms > b.MaxMS {
			b.MaxMS = ms
		}
		if ms >= c.slowMS {
			b.Slow++
		}
	}
	c.trim(idx)
}

// trim drops buckets that fall outside the rolling window. The current bucket
// (Start == idx) is never trimmed.
func (c *TimeSeriesCollector) trim(idx int64) {
	minStart := (idx - int64(c.window) + 1) * c.bucket
	cut := 0
	for cut < len(c.buckets) && c.buckets[cut].Start < minStart {
		cut++
	}
	if cut > 0 {
		c.buckets = append(c.buckets[:0], c.buckets[cut:]...)
	}
}

// Snapshot returns a copy of the window with spike flags computed against the
// median error and slow-query rates. A bucket is a spike when its count is at
// least three times the median and at least three in absolute terms.
func (c *TimeSeriesCollector) Snapshot() TimeSeriesSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	buckets := append([]TimeBucket(nil), c.buckets...)
	errs := make([]uint64, len(buckets))
	slows := make([]uint64, len(buckets))
	for i, b := range buckets {
		errs[i] = b.Errors
		slows[i] = b.Slow
	}
	medErr := medianCount(errs)
	medSlow := medianCount(slows)
	for i := range buckets {
		b := &buckets[i]
		b.ErrSpike = b.Errors >= 3 && float64(b.Errors) >= 3*medErr
		b.SlowSpike = b.Slow >= 3 && float64(b.Slow) >= 3*medSlow
	}
	return TimeSeriesSnapshot{BucketSeconds: c.bucket, SlowMS: c.slowMS, Buckets: buckets}
}

func medianCount(vals []uint64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := append([]uint64(nil), vals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	if n%2 == 1 {
		return float64(sorted[n/2])
	}
	return float64(sorted[n/2-1]+sorted[n/2]) / 2
}
