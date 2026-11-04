package web

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"local/sql-monitor/internal/analyze"
	"local/sql-monitor/internal/monitor"
)

func TestAnalyzeEndpointsDisabled(t *testing.T) {
	_, h := httpFixture(t)
	w := request(h, "GET", "/api/analyze/status")
	if w.Code != 200 {
		t.Fatalf("status code %d", w.Code)
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Enabled {
		t.Fatalf("expected disabled: %s %v", w.Body.String(), err)
	}
	w = request(h, "POST", "/api/analyze")
	if w.Code != 404 {
		t.Fatalf("POST without analyzer: %d %s", w.Code, w.Body.String())
	}
}

func TestAnalyzeNoHotspots(t *testing.T) {
	m, h := httpFixture(t)
	m.SetAggregateStore(monitor.NewAggregateStore(200))
	a := analyze.New([]string{"sh", "-c", "cat"}, "", 5*time.Second, "demo", "local", 5, nil, io.Discard)
	h.SetAnalyzer(a)

	w := request(h, "POST", "/api/analyze")
	if w.Code != 200 {
		t.Fatalf("no-hotspot trigger: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Started bool   `json:"started"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Started || body.Reason != "no_hotspots" {
		t.Fatalf("no-hotspot body: %s %v", w.Body.String(), err)
	}
}

func TestAnalyzeTriggerAndStatus(t *testing.T) {
	m, h := httpFixture(t)
	m.SetAggregateStore(monitor.NewAggregateStore(200))
	m.Consume([]byte("{\"msg\":\"SQL Error\",\"sql\":\"SELECT 1\",\"source\":\"a.go:1\"}\n"))
	a := analyze.New([]string{"sh", "-c", "cat"}, "", 5*time.Second, "demo", "local", 5, nil, io.Discard)
	h.SetAnalyzer(a)

	w := request(h, "GET", "/api/analyze/status")
	var st struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &st)
	if !st.Enabled {
		t.Fatal("expected analyzer enabled")
	}

	w = request(h, "POST", "/api/analyze")
	if w.Code != 202 {
		t.Fatalf("trigger: %d %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		w = request(h, "GET", "/api/analyze/status")
		var full struct {
			Status analyze.Status `json:"status"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &full)
		if !full.Status.Running {
			if full.Status.Error != "" || full.Status.Result == "" {
				t.Fatalf("analysis failed: %+v", full.Status)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("analysis never finished")
}
