package analyze

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"local/sql-monitor/internal/monitor"
)

type fakeSender struct {
	mu   sync.Mutex
	text []string
}

func (f *fakeSender) SendText(_ context.Context, text string, _ io.Writer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.text = append(f.text, text)
	return nil
}

func TestFromAggregatesFiltersAndRanks(t *testing.T) {
	groups := []monitor.SQLAggregate{
		{Fingerprint: "clean", Source: "a.go:1", Count: 50},                              // no anomaly, dropped
		{Fingerprint: "err", Source: "b.go:2", Count: 10, Errors: 4, Score: 40},          // rank 1
		{Fingerprint: "n1", Source: "c.go:3", Count: 5, N1: true, N1Max: 7, Score: 8},    // rank 2
		{Fingerprint: "slow", Source: "d.go:4", Count: 3, Slow: 1, MaxMS: 500, Score: 2}, // rank 3
	}
	hot := FromAggregates(groups, 5)
	if len(hot) != 3 {
		t.Fatalf("expected 3 hotspots, got %d", len(hot))
	}
	if hot[0].Fingerprint != "err" || hot[1].Fingerprint != "n1" || hot[2].Fingerprint != "slow" {
		t.Fatalf("wrong ranking: %+v", hot)
	}
	if !hot[1].N1 || hot[1].N1Max != 7 {
		t.Fatalf("N1 evidence lost: %+v", hot[1])
	}
	if len(FromAggregates(groups, 2)) != 2 {
		t.Fatal("topN not applied")
	}
}

func TestFromSummarySkipsClean(t *testing.T) {
	groups := []monitor.SQLSummaryGroup{
		{Fingerprint: "clean", Source: "a.go:1", Count: 9},
		{Fingerprint: "err", Source: "b.go:2", Count: 2, Errors: 2},
	}
	hot := FromSummary(groups, 5)
	if len(hot) != 1 || hot[0].Fingerprint != "err" {
		t.Fatalf("unexpected: %+v", hot)
	}
}

func TestBuildPromptContainsFingerprintsNotSQL(t *testing.T) {
	hot := []Hotspot{{Fingerprint: "deadbeef", Source: "svc.go:12", Errors: 3, Slow: 1, MaxMS: 300, Reason: "执行错误 3 次"}}
	p := BuildPrompt(hot, "demo", "k8s dev")
	for _, want := range []string{"deadbeef", "svc.go:12", "执行错误 3 次", "demo", "k8s dev"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q: %s", want, p)
		}
	}
	if strings.Contains(p, "SELECT") || strings.Contains(p, "secret") {
		t.Fatal("prompt leaked raw SQL")
	}
}

func TestExecuteEchoesStdin(t *testing.T) {
	a := New([]string{"sh", "-c", "cat"}, "", time.Second, "demo", "local", 5, nil, io.Discard)
	out, err := a.execute(context.Background(), "hello\nworld")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello\nworld" {
		t.Fatalf("stdout not echoed: %q", out)
	}
}

func TestTriggerRunsAndSends(t *testing.T) {
	sender := &fakeSender{}
	a := New([]string{"sh", "-c", "cat"}, "", 5*time.Second, "demo", "local", 5, sender, io.Discard)
	groups := []monitor.SQLAggregate{{Fingerprint: "x", Source: "s.go:1", Errors: 2, Score: 20}}
	if !a.TriggerAggregates(groups) {
		t.Fatal("trigger refused")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !a.Status().Running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	st := a.Status()
	if st.Running || st.Error != "" || st.Result == "" || !st.SentToFeishu {
		t.Fatalf("unexpected status: %+v", st)
	}
	if len(sender.text) != 1 || !strings.Contains(sender.text[0], "x") {
		t.Fatalf("sender text wrong: %+v", sender.text)
	}
}

func TestTriggerRejectsWhileRunning(t *testing.T) {
	a := New([]string{"sh", "-c", "sleep 0.3"}, "", 5*time.Second, "demo", "local", 5, nil, io.Discard)
	groups := []monitor.SQLAggregate{{Fingerprint: "x", Source: "s.go:1", Errors: 2, Score: 20}}
	if !a.TriggerAggregates(groups) {
		t.Fatal("first trigger refused")
	}
	if a.TriggerAggregates(groups) {
		t.Fatal("second trigger should be refused while running")
	}
	if a.TriggerAggregates(nil) {
		t.Fatal("empty trigger should be refused")
	}
}
