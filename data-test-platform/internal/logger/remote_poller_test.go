package logger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"data-test-platform/internal/config"
)

func TestRemoteLogPollerParsingAndDedupV2(t *testing.T) {
	// Mock workflow log server returning the new { "logs": [ ... ] } payload format
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		resp := map[string]interface{}{
			"logs": []map[string]interface{}{
				{
					"time":     "2026-09-28T17:01:10.624838197+08:00",
					"level":    "",
					"service":  "data-service",
					"instance": "data-service-dev3-79f5994f89-zx7c4",
					"traceId":  "",
					"message":  "[2026-09-28T17:01:10+08:00] \u001b[90mDEBUG\u001b[0m [data-service/zx7c4/v20260920-2f93b04] [] worker/data_hub_events_worker.go:108 - [data-hub-kafka] step=skip_missing episode_id=eps-123",
					"fields": map[string]interface{}{
						"_namespace_":      "ns-cloud-dev",
						"_pod_name_":       "data-service-dev3-79f5994f89-zx7c4",
						"_container_name_": "data-service",
						"_source_":         "stdout",
						"_time_":           "2026-09-28T17:01:10.624838197+08:00",
					},
				},
				{
					"time":     "2026-09-28T17:01:10.624821358+08:00",
					"level":    "",
					"service":  "data-service",
					"instance": "data-service-dev3-79f5994f89-zx7c4",
					"traceId":  "",
					"message":  "[2026-09-28T17:01:10+08:00] \u001b[90mDEBUG\u001b[0m [data-service/zx7c4/v20260920-2f93b04] [] data/gorm_logger.go:140 - source=internal/data/episode.go:249 | SQL Query | elapsed=\u001b[32m14.601496ms\u001b[0m | rows=0 | sql=SELECT * FROM `tb_episode` WHERE `id` = '1'",
					"fields": map[string]interface{}{
						"_namespace_":      "ns-cloud-dev",
						"_pod_name_":       "data-service-dev3-79f5994f89-zx7c4",
						"_container_name_": "data-service",
						"_source_":         "stdout",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.NewDefaultConfig()
	cfg.RemoteLog.WorkflowURL = server.URL
	cfg.RemoteLog.AuthToken = "test-token"
	cfg.RemoteLog.LogSource = "dev"
	// 默认配置不再内置具体环境（已改为 profile 驱动），测试自己声明
	cfg.RemoteLog.Environments = map[string]config.RemoteLogEnvConfig{
		"dev": {Environment: "ns-dev", Namespace: "ns-dev", Service: "svc"},
	}

	parser := NewLogParser(10 * time.Millisecond)
	hub := NewHub(parser)
	poller := NewRemoteLogPoller(cfg, hub)

	// First poll
	poller.pollOnce()

	time.Sleep(50 * time.Millisecond)
	logs := hub.GetRecentLogs(10, "", "", false, false)
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs ingested, got %d", len(logs))
	}

	// Verify query body parameters
	if receivedBody["environment"] != "ns-dev" {
		t.Errorf("expected request environment 'ns-dev', got '%v'", receivedBody["environment"])
	}
	if receivedBody["service"] != "svc" {
		t.Errorf("expected request service 'svc', got '%v'", receivedBody["service"])
	}
	if receivedBody["queryMode"] != "keyword" {
		t.Errorf("expected request queryMode 'keyword', got '%v'", receivedBody["queryMode"])
	}
	if receivedBody["timeRange"] != "realtime" {
		t.Errorf("expected request timeRange 'realtime', got '%v'", receivedBody["timeRange"])
	}

	// Verify enriched metadata & parsing
	var sqlFound, nonSqlFound bool
	for _, l := range logs {
		if l.PodName != "data-service-dev3-79f5994f89-zx7c4" {
			t.Errorf("unexpected pod name: %s", l.PodName)
		}
		if l.Namespace != "ns-cloud-dev" {
			t.Errorf("unexpected namespace: %s", l.Namespace)
		}
		if l.LogSource != "dev" {
			t.Errorf("unexpected log source: %s", l.LogSource)
		}
		if l.IsSQL {
			sqlFound = true
			if l.SQL != "SELECT * FROM `tb_episode` WHERE `id` = '1'" {
				t.Errorf("unexpected SQL parsed: %s", l.SQL)
			}
			if l.Source != "internal/data/episode.go:249" {
				t.Errorf("unexpected SQL source caller: %s", l.Source)
			}
			if l.DurationMs < 14 || l.DurationMs > 15 {
				t.Errorf("unexpected DurationMs: %f", l.DurationMs)
			}
		} else {
			nonSqlFound = true
		}
	}

	if !sqlFound || !nonSqlFound {
		t.Errorf("expected 1 SQL and 1 non-SQL log, sqlFound=%v, nonSqlFound=%v", sqlFound, nonSqlFound)
	}

	// Second poll: duplicate messages should be deduplicated
	poller.pollOnce()
	logsAfterDedup := hub.GetRecentLogs(10, "", "", false, false)
	if len(logsAfterDedup) != 2 {
		t.Errorf("expected deduplication to keep log count at 2, got %d", len(logsAfterDedup))
	}

	// Test connection function with new params
	testRes, err := poller.TestConnection(TestConnectionParams{
		WorkflowURL: server.URL,
		AuthToken:   "test-token",
		Environment: "ns-cloud-dev",
		Service:     "data-service",
	})
	if err != nil {
		t.Fatalf("test connection failed: %v", err)
	}
	if !testRes.Success {
		t.Errorf("expected test connection to succeed: %s", testRes.Message)
	}
	if testRes.Count != 2 {
		t.Errorf("expected 2 test items, got %d", testRes.Count)
	}
}

func TestLiveObservabilityConnection(t *testing.T) {
	cfg := config.NewDefaultConfig()
	hub := NewHub(NewLogParser(200 * time.Millisecond))
	poller := NewRemoteLogPoller(cfg, hub)

	res, err := poller.TestConnection(TestConnectionParams{
		WorkflowURL: "https://devops.x2robot.com/api/v1/observability/logs/query",
		AuthToken:   "dummy-token-to-verify-connection",
		Environment: "ns-cloud-dev",
		Service:     "data-service",
	})
	if err != nil {
		t.Fatalf("unexpected connection error: %v", err)
	}
	// We expect HTTP 401 or response from endpoint, NOT network EOF!
	t.Logf("Response received from devops endpoint: success=%v, msg=%s, duration=%dms", res.Success, res.Message, res.DurationMs)
}

func TestRemoteLogPollerSearchHistory(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()

		page := int(body["page"].(float64))
		const pageSize = 500
		items := []map[string]interface{}{}
		if page <= 2 {
			// page 2 holds the older 500 entries, page 1 the newer 500;
			// like the real API, items within a page are newest-first
			for i := pageSize - 1; i >= 0; i-- {
				idx := (2-page)*pageSize + i
				ts := time.Unix(1790000000+int64(idx), 0).UTC().Format(time.RFC3339Nano)
				items = append(items, map[string]interface{}{
					"time":     ts,
					"service":  "data-service",
					"instance": "pod-a",
					"message":  fmt.Sprintf("log-line-%d", idx),
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"logs": items})
	}))
	defer server.Close()

	cfg := config.NewDefaultConfig()
	cfg.RemoteLog.WorkflowURL = server.URL
	cfg.RemoteLog.AuthToken = "test-token"
	cfg.RemoteLog.LogSource = "test"
	cfg.RemoteLog.Environments = map[string]config.RemoteLogEnvConfig{
		"test": {Environment: "ns-test", Namespace: "ns-test", Service: "svc"},
	}

	parser := NewLogParser(10 * time.Millisecond)
	hub := NewHub(parser)
	poller := NewRemoteLogPoller(cfg, hub)

	res, err := poller.SearchHistory(SearchParams{
		TimeRange: "24h",
		QueryMode: "keyword",
		Keyword:   "job-xxx DispatchJob",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("search not successful: %s", res.Message)
	}
	if res.Ingested != 1000 {
		t.Fatalf("expected 1000 ingested, got %d (fetched=%d, pages=%d)", res.Ingested, res.Fetched, res.Pages)
	}
	if res.Truncated {
		t.Errorf("unexpected truncation")
	}

	// 等待 broadcastChan 排空后再校验 hub 缓冲
	waitHubLen := func(want int) int {
		deadline := time.Now().Add(3 * time.Second)
		for {
			got := len(hub.GetRecentLogs(0, "", "", false, false))
			if got == want || time.Now().After(deadline) {
				return got
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if got := waitHubLen(1000); got != 1000 {
		t.Fatalf("expected 1000 logs in hub, got %d", got)
	}
	logs := hub.GetRecentLogs(0, "", "", false, false)
	// Chronological order: oldest first
	if logs[0].Message != "log-line-0" || logs[999].Message != "log-line-999" {
		t.Errorf("unexpected ordering: first=%q last=%q", logs[0].Message, logs[999].Message)
	}

	mu.Lock()
	bodyList := append([]map[string]interface{}{}, bodies...)
	mu.Unlock()

	if len(bodyList) != 3 {
		t.Fatalf("expected 3 page requests, got %d", len(bodyList))
	}
	first := bodyList[0]
	if first["keyword"] != "job-xxx DispatchJob" {
		t.Errorf("expected keyword passthrough, got %v", first["keyword"])
	}
	st, ok := first["startTime"].(string)
	if !ok || st == "" {
		t.Errorf("expected startTime as non-empty string, got %v (%T)", first["startTime"], first["startTime"])
	}
	et, ok := first["endTime"].(string)
	if !ok || et == "" {
		t.Errorf("expected endTime as non-empty string, got %v (%T)", first["endTime"], first["endTime"])
	}
	if _, exists := first["timeRange"]; exists {
		t.Errorf("history search must not send timeRange, got %v", first["timeRange"])
	}
	if first["queryMode"] != "keyword" {
		t.Errorf("expected queryMode keyword, got %v", first["queryMode"])
	}

	// Realtime poll afterwards must not duplicate search results
	poller.pollOnce()
	logsAfter := hub.GetRecentLogs(0, "", "", false, false)
	if len(logsAfter) != 1000 {
		t.Errorf("expected dedup to keep 1000 logs after realtime poll, got %d", len(logsAfter))
	}
}

func TestRemoteLogPollerSearchHistoryCustomRange(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"logs": []map[string]interface{}{}})
	}))
	defer server.Close()

	cfg := config.NewDefaultConfig()
	cfg.RemoteLog.WorkflowURL = server.URL
	cfg.RemoteLog.AuthToken = "test-token"
	cfg.RemoteLog.LogSource = "dev"
	// 默认配置不再内置具体环境（已改为 profile 驱动），测试自己声明
	cfg.RemoteLog.Environments = map[string]config.RemoteLogEnvConfig{
		"dev": {Environment: "ns-dev", Namespace: "ns-dev", Service: "svc"},
	}

	poller := NewRemoteLogPoller(cfg, NewHub(NewLogParser(10*time.Millisecond)))

	// Custom [start, end] window must be passed through as unix-second strings
	res, err := poller.SearchHistory(SearchParams{
		QueryMode: "keyword",
		StartTime: 1791439680,
		EndTime:   1792638480,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("search not successful: %s", res.Message)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("expected 1 request, got %d", len(bodies))
	}
	if bodies[0]["startTime"] != "1791439680" {
		t.Errorf("expected startTime '1791439680', got %v (%T)", bodies[0]["startTime"], bodies[0]["startTime"])
	}
	if bodies[0]["endTime"] != "1792638480" {
		t.Errorf("expected endTime '1792638480', got %v (%T)", bodies[0]["endTime"], bodies[0]["endTime"])
	}

	// Invalid custom range (start >= end) must be rejected
	if _, err := poller.SearchHistory(SearchParams{StartTime: 200, EndTime: 100}); err == nil {
		t.Errorf("expected error for start >= end custom range")
	}
}
