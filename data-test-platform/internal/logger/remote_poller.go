package logger

import (
	"bytes"
	"crypto/md5"
	"data-test-platform/internal/pkg/directnet"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"data-test-platform/internal/config"
)

// RemotePollerStatus reports the status of remote log polling
type RemotePollerStatus struct {
	ActiveSource  string    `json:"active_source"` // "local", "dev", "test"
	IsPolling     bool      `json:"is_polling"`
	WorkflowURL   string    `json:"workflow_url"`
	Environment   string    `json:"environment"`
	Namespace     string    `json:"namespace"`
	Service       string    `json:"service"`
	Source        string    `json:"source"`
	ProjectIDs    []string  `json:"project_ids"`
	HasToken      bool      `json:"has_token"`
	LastPollAt    time.Time `json:"last_poll_at"`
	LastSuccessAt time.Time `json:"last_success_at"`
	LastError     string    `json:"last_error"`
	LogsFetched   uint64    `json:"logs_fetched"`
}

// RemoteLogTestResult provides the test fetch result
type RemoteLogTestResult struct {
	Success     bool        `json:"success"`
	Message     string      `json:"message"`
	Count       int         `json:"count"`
	SampleEntry interface{} `json:"sample_entry,omitempty"`
	DurationMs  int64       `json:"duration_ms"`
}

// TestConnectionParams holds parameters for testing workflow log connectivity
type TestConnectionParams struct {
	WorkflowURL string   `json:"workflow_url"`
	AuthToken   string   `json:"auth_token"`
	Environment string   `json:"environment"`
	Namespace   string   `json:"namespace"`
	Service     string   `json:"service"`
	Source      string   `json:"source"`
	QueryMode   string   `json:"query_mode"`
	TimeRange   string   `json:"time_range"`
	SLSProject  string   `json:"sls_project"`
	ProjectIDs  []string `json:"project_ids"`
}

// RemoteLogItem represents a cloud log record from the workflow search API
type RemoteLogItem struct {
	Time     string                 `json:"time"`
	Level    string                 `json:"level"`
	Service  string                 `json:"service"`
	Instance string                 `json:"instance"`
	TraceID  string                 `json:"traceId"`
	Message  string                 `json:"message"`
	Fields   map[string]interface{} `json:"fields"`

	// Backward compatibility with legacy workflow log fields:
	TimestampMS   int64             `json:"timestamp_ms"`
	ProjectID     string            `json:"project_id"`
	ProjectName   string            `json:"project_name"`
	AppName       string            `json:"app_name"`
	Namespace     string            `json:"namespace"`
	ContainerName string            `json:"container_name"`
	ImageName     string            `json:"image_name"`
	PodName       string            `json:"pod_name"`
	ContainerIP   string            `json:"container_ip"`
	Source        string            `json:"source"`
	Metadata      map[string]string `json:"metadata"`
}

// parseRemoteLogResponse extracts log items from any supported response format
func parseRemoteLogResponse(respBytes []byte) ([]RemoteLogItem, error) {
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Logs    []RemoteLogItem `json:"logs"`
		Data    json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		var directList []RemoteLogItem
		if err2 := json.Unmarshal(respBytes, &directList); err2 == nil {
			return directList, nil
		}
		return nil, fmt.Errorf("解析 JSON 响应失败: %v", err)
	}

	if envelope.Code != 0 && envelope.Code != 200 && envelope.Message != "" {
		return nil, fmt.Errorf("远端错误 (code=%d): %s", envelope.Code, envelope.Message)
	}

	if len(envelope.Logs) > 0 {
		return envelope.Logs, nil
	}

	if len(envelope.Data) > 0 {
		// Data might be []RemoteLogItem
		var dataList []RemoteLogItem
		if err := json.Unmarshal(envelope.Data, &dataList); err == nil && len(dataList) > 0 {
			return dataList, nil
		}

		// Data might be { "logs": [...] } or { "list": [...] }
		var dataObj struct {
			Logs []RemoteLogItem `json:"logs"`
			List []RemoteLogItem `json:"list"`
		}
		if err := json.Unmarshal(envelope.Data, &dataObj); err == nil {
			if len(dataObj.Logs) > 0 {
				return dataObj.Logs, nil
			}
			if len(dataObj.List) > 0 {
				return dataObj.List, nil
			}
		}
	}

	return envelope.Logs, nil
}

// RemoteLogPoller continuously polls cloud service logs from the workflow API
type RemoteLogPoller struct {
	mu            sync.RWMutex
	cfg           *config.Config
	hub           *Hub
	client        *http.Client
	stopChan      chan struct{}
	running       bool
	searching     bool // true while a history search is in flight; realtime polling pauses
	lastStartTime int64
	seenKeys      map[string]int64
	lastPollAt    time.Time
	lastSuccessAt time.Time
	lastError     string
	logsFetched   uint64
	lastTargetEnv string
}

// NewRemoteLogPoller creates a new remote log poller
func NewRemoteLogPoller(cfg *config.Config, hub *Hub) *RemoteLogPoller {
	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			if directnet.IsDirect(req.URL.Hostname()) {
				return nil, nil // 内部域名直连，绕过系统代理 (Clash/Surge)
			}
			return http.ProxyFromEnvironment(req)
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &RemoteLogPoller{
		cfg:      cfg,
		hub:      hub,
		seenKeys: make(map[string]int64),
		client: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		},
	}
}

// Start launches the background polling goroutine
func (p *RemoteLogPoller) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.stopChan = make(chan struct{})
	p.mu.Unlock()

	go p.pollLoop()
}

// Stop terminates the polling goroutine
func (p *RemoteLogPoller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	p.running = false
	close(p.stopChan)
}

// ResetWindow resets the last start time window (e.g. after environment switch)
func (p *RemoteLogPoller) ResetWindow() {
	p.mu.Lock()
	p.lastStartTime = 0
	p.seenKeys = make(map[string]int64)
	p.lastError = ""
	p.mu.Unlock()
}

// GetStatus returns the current polling status
func (p *RemoteLogPoller) GetStatus() RemotePollerStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	targetEnv, envCfg, isRemote := p.cfg.GetActiveRemoteLogTarget()
	rlCfg := p.cfg.GetRemoteLogConfig()

	service := envCfg.Service
	if service == "" {
		service = rlCfg.Service
	}
	source := envCfg.Source
	if source == "" {
		source = rlCfg.Source
	}

	return RemotePollerStatus{
		ActiveSource:  targetEnv,
		IsPolling:     p.running && isRemote,
		WorkflowURL:   rlCfg.WorkflowURL,
		Environment:   envCfg.GetEnvironment(),
		Namespace:     envCfg.GetEnvironment(),
		Service:       service,
		Source:        source,
		ProjectIDs:    envCfg.ProjectIDs,
		HasToken:      strings.TrimSpace(rlCfg.AuthToken) != "",
		LastPollAt:    p.lastPollAt,
		LastSuccessAt: p.lastSuccessAt,
		LastError:     p.lastError,
		LogsFetched:   atomic.LoadUint64(&p.logsFetched),
	}
}

func (p *RemoteLogPoller) pollLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.pollOnce()

			// Adjust ticker if user modified poll interval
			intervalMs := p.cfg.GetRemoteLogConfig().PollIntervalMs
			if intervalMs < 1000 {
				intervalMs = 1000
			}
			newInterval := time.Duration(intervalMs) * time.Millisecond
			ticker.Reset(newInterval)
		}
	}
}

// remoteQuery describes a single page request against the workflow log API
type remoteQuery struct {
	workflowURL string
	token       string
	environment string
	service     string
	source      string
	queryMode   string
	timeRange   string // "realtime" for polling; empty when startTime/endTime set
	keyword     string
	startTime   int64 // unix seconds; history search window start
	endTime     int64 // unix seconds; history search window end
	page        int
	pageSize    int
}

// SearchParams holds parameters for one-shot history log search
type SearchParams struct {
	TimeRange  string `json:"time_range"`  // "15m", "1h", "24h", "7d"; ignored when custom window set
	QueryMode  string `json:"query_mode"`  // "keyword" or "sls"
	Keyword    string `json:"keyword"`     // raw keyword / SLS query syntax, passed through as-is
	StartTime  int64  `json:"start_time"`  // custom window start (unix seconds); overrides time_range when > 0
	EndTime    int64  `json:"end_time"`    // custom window end (unix seconds); overrides time_range when > 0
	MaxResults int    `json:"max_results"` // upper bound of entries to fetch, default 10000
}

// SearchResult reports the outcome of a history search
type SearchResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	Fetched    int    `json:"fetched"`   // raw items returned by remote
	Ingested   int    `json:"ingested"`  // entries ingested after dedup
	Pages      int    `json:"pages"`     // pages fetched
	Truncated  bool   `json:"truncated"` // true if max_results limit reached
	DurationMs int64  `json:"duration_ms"`
}

// fetchRemoteLogs performs a single page request and returns parsed log items.
// It is shared by realtime polling and one-shot history search.
func (p *RemoteLogPoller) fetchRemoteLogs(q remoteQuery) ([]RemoteLogItem, error) {
	reqBody := map[string]interface{}{
		"source":      q.source,
		"environment": q.environment,
		"service":     q.service,
		"queryMode":   q.queryMode,
		"page":        q.page,
		"pageSize":    q.pageSize,
	}
	if q.startTime > 0 && q.endTime > 0 {
		// History search window per DevOps API: startTime/endTime are unix
		// second strings; timeRange is not sent in this mode.
		reqBody["startTime"] = strconv.FormatInt(q.startTime, 10)
		reqBody["endTime"] = strconv.FormatInt(q.endTime, 10)
	} else {
		reqBody["timeRange"] = q.timeRange
	}
	// keyword is only included when non-empty (realtime polling never sends it)
	if strings.TrimSpace(q.keyword) != "" {
		reqBody["keyword"] = q.keyword
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", q.workflowURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", q.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("认证失败 (401 Unauthorized): 请检查 Authorization Token")
		}
		return nil, fmt.Errorf("远端服务响应 HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	return parseRemoteLogResponse(respBytes)
}

// ingestRemoteItems parses, enriches, dedupes and ingests a page of remote log
// items into the hub. It is shared by realtime polling and history search, so
// the dedup registry is unified: entries fetched by a history search are never
// re-ingested by subsequent realtime polls. Returns ingested count and the max
// log timestamp (ms) seen.
func (p *RemoteLogPoller) ingestRemoteItems(items []RemoteLogItem, targetEnv, environment string, now time.Time) (int, int64) {
	var maxTS int64
	var ingested int
	nowUnix := now.Unix()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Process logs in chronological order (oldest first: reverse list)
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]

		// Resolve log message
		rawMsg := item.Message
		if rawMsg == "" && item.Fields != nil {
			if c, ok := item.Fields["content"].(string); ok && c != "" {
				rawMsg = c
			}
		}

		// Resolve pod name / instance
		podName := item.Instance
		if podName == "" {
			podName = item.PodName
		}
		if podName == "" && item.Fields != nil {
			if p, ok := item.Fields["_pod_name_"].(string); ok {
				podName = p
			}
		}

		// Resolve timestamp
		var logTime time.Time
		if item.Time != "" {
			if t, err := time.Parse(time.RFC3339Nano, item.Time); err == nil {
				logTime = t
			} else if t, err := time.Parse(time.RFC3339, item.Time); err == nil {
				logTime = t
			}
		}
		if logTime.IsZero() && item.TimestampMS > 0 {
			logTime = time.UnixMilli(item.TimestampMS)
		}
		if logTime.IsZero() && item.Fields != nil {
			if tStr, ok := item.Fields["_time_"].(string); ok && tStr != "" {
				if t, err := time.Parse(time.RFC3339Nano, tStr); err == nil {
					logTime = t
				}
			}
		}
		if logTime.IsZero() {
			logTime = now
		}

		tsMilli := logTime.UnixMilli()
		if tsMilli > maxTS {
			maxTS = tsMilli
		}

		// Deduplication
		hash := md5.Sum([]byte(rawMsg))
		key := fmt.Sprintf("%d_%s_%s", tsMilli, podName, hex.EncodeToString(hash[:]))
		if _, exists := p.seenKeys[key]; exists {
			continue
		}
		p.seenKeys[key] = nowUnix

		// Parse log line
		entry := p.hub.Parser().ParseLine(rawMsg)
		if entry == nil {
			entry = &LogEntry{
				Timestamp: logTime,
				TimeStr:   logTime.Format("15:04:05.000"),
				Level:     "INFO",
				Raw:       rawMsg,
				Message:   rawMsg,
			}
		}

		// Resolve container & namespace
		containerName := item.ContainerName
		if containerName == "" && item.Fields != nil {
			if cn, ok := item.Fields["_container_name_"].(string); ok {
				containerName = cn
			}
		}
		if containerName == "" {
			containerName = item.Service
		}

		logNamespace := item.Namespace
		if logNamespace == "" && item.Fields != nil {
			if ns, ok := item.Fields["_namespace_"].(string); ok {
				logNamespace = ns
			}
		}
		if logNamespace == "" {
			logNamespace = environment
		}

		entry.PodName = podName
		entry.ContainerName = containerName
		entry.Namespace = logNamespace
		entry.ProjectName = item.ProjectName
		entry.LogSource = targetEnv
		entry.Timestamp = logTime
		entry.TimeStr = logTime.Format("15:04:05.000")

		if item.TraceID != "" {
			entry.TraceID = item.TraceID
		}
		if entry.Source == "" {
			entry.Source = podName
		}

		p.hub.IngestEntry(entry)
		atomic.AddUint64(&p.logsFetched, 1)
		ingested++
	}

	// Periodic purge of old keys (> 5 minutes)
	if len(p.seenKeys) > 5000 {
		for k, t := range p.seenKeys {
			if nowUnix-t > 300 {
				delete(p.seenKeys, k)
			}
		}
	}

	return ingested, maxTS
}

func (p *RemoteLogPoller) pollOnce() {
	// While a history search is running, realtime polling pauses so the search
	// result stream is not interleaved with incremental page-1 fetches.
	p.mu.RLock()
	searching := p.searching
	p.mu.RUnlock()
	if searching {
		return
	}

	targetEnv, envCfg, isRemote := p.cfg.GetActiveRemoteLogTarget()
	if !isRemote {
		p.mu.Lock()
		p.lastTargetEnv = targetEnv
		p.mu.Unlock()
		return
	}

	p.mu.Lock()
	if p.lastTargetEnv != targetEnv {
		p.lastTargetEnv = targetEnv
		p.lastStartTime = 0
		p.seenKeys = make(map[string]int64)
	}
	p.lastPollAt = time.Now()
	p.mu.Unlock()

	rlCfg := p.cfg.GetRemoteLogConfig()
	token := strings.TrimSpace(rlCfg.AuthToken)
	if token == "" {
		p.mu.Lock()
		p.lastError = "未配置远端日志 Authorization Token，请在设置中配置"
		p.mu.Unlock()
		return
	}

	workflowURL := rlCfg.WorkflowURL
	if workflowURL == "" {
		p.mu.Lock()
		p.lastError = "未配置远端日志查询地址 workflow_url，请在项目配置中提供"
		p.mu.Unlock()
		return
	}

	environment := envCfg.GetEnvironment()

	service := envCfg.Service
	if service == "" {
		service = rlCfg.Service
	}
	if service == "" {
		service = rlCfg.Service
	}

	source := envCfg.Source
	if source == "" {
		source = rlCfg.Source
	}
	if source == "" {
		source = "default"
	}

	queryMode := rlCfg.QueryMode
	if queryMode == "" {
		queryMode = "keyword"
	}

	timeRange := rlCfg.TimeRange
	if timeRange == "" {
		timeRange = "realtime"
	}

	pageSize := rlCfg.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	now := time.Now().UTC()

	logs, err := p.fetchRemoteLogs(remoteQuery{
		workflowURL: workflowURL,
		token:       token,
		environment: environment,
		service:     service,
		source:      source,
		queryMode:   queryMode,
		timeRange:   timeRange,
		page:        1,
		pageSize:    pageSize,
	})
	if err != nil {
		p.mu.Lock()
		p.lastError = err.Error()
		p.mu.Unlock()
		return
	}

	p.mu.Lock()
	p.lastError = ""
	p.lastSuccessAt = time.Now()
	p.mu.Unlock()

	if len(logs) == 0 {
		p.mu.Lock()
		p.lastStartTime = now.Add(-5 * time.Second).Unix()
		p.mu.Unlock()
		return
	}

	ingested, maxTS := p.ingestRemoteItems(logs, targetEnv, environment, now)
	_ = ingested

	p.mu.Lock()
	// Advance lastStartTime with 2 seconds buffer for safety
	if maxTS > 0 {
		p.lastStartTime = (maxTS / 1000) - 2
	} else {
		p.lastStartTime = now.Add(-5 * time.Second).Unix()
	}
	p.mu.Unlock()
}

// TestConnection verifies credentials and returns sample data
func (p *RemoteLogPoller) TestConnection(params TestConnectionParams) (*RemoteLogTestResult, error) {
	workflowURL := params.WorkflowURL
	if workflowURL == "" {
		return &RemoteLogTestResult{
			Success: false,
			Message: "查询地址 workflow_url 不能为空",
		}, nil
	}
	token := strings.TrimSpace(params.AuthToken)
	if token == "" {
		return &RemoteLogTestResult{
			Success: false,
			Message: "Authorization Token 不能为空",
		}, nil
	}

	environment := params.Environment
	if environment == "" {
		environment = params.Namespace
	}
	if environment == "" {
		return &RemoteLogTestResult{
			Success: false,
			Message: "环境 environment/namespace 不能为空",
		}, nil
	}

	service := params.Service

	source := params.Source
	if source == "" {
		source = "default"
	}

	queryMode := params.QueryMode
	if queryMode == "" {
		queryMode = "keyword"
	}

	timeRange := params.TimeRange
	if timeRange == "" {
		timeRange = "realtime"
	}

	reqBody := map[string]interface{}{
		"source":      source,
		"environment": environment,
		"service":     service,
		"queryMode":   queryMode,
		"page":        1,
		"pageSize":    10,
		"timeRange":   timeRange,
	}

	jsonBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", workflowURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)

	startReq := time.Now()
	resp, err := p.client.Do(req)
	duration := time.Since(startReq).Milliseconds()

	if err != nil {
		return &RemoteLogTestResult{
			Success:    false,
			Message:    fmt.Sprintf("网络连接失败: %v", err),
			DurationMs: duration,
		}, nil
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("HTTP 状态码错误: %d", resp.StatusCode)
		if resp.StatusCode == http.StatusUnauthorized {
			msg = "认证失败 (401 Unauthorized): 请检查 Authorization Token"
		}
		return &RemoteLogTestResult{
			Success:    false,
			Message:    msg,
			DurationMs: duration,
		}, nil
	}

	logs, err := parseRemoteLogResponse(respBytes)
	if err != nil {
		return &RemoteLogTestResult{
			Success:    false,
			Message:    err.Error(),
			DurationMs: duration,
		}, nil
	}

	sample := interface{}(nil)
	if len(logs) > 0 {
		sample = logs[0]
	}

	return &RemoteLogTestResult{
		Success:     true,
		Message:     fmt.Sprintf("连接成功！成功拉取到 %d 条云端日志", len(logs)),
		Count:       len(logs),
		SampleEntry: sample,
		DurationMs:  duration,
	}, nil
}

// allowedSearchTimeRanges maps history search windows to their durations
var allowedSearchTimeRanges = map[string]time.Duration{
	"15m": 15 * time.Minute,
	"1h":  1 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

// SearchHistory performs a one-shot history log search: it clears the hub,
// resets the realtime dedup window, then paged-fetches (500 per page) all logs
// in the requested time window up to maxResults and ingests them through the
// shared parse/dedup/ingest pipeline. Realtime polling continues afterwards,
// appending new logs to the tail of the search results.
func (p *RemoteLogPoller) SearchHistory(params SearchParams) (*SearchResult, error) {
	start := time.Now()

	targetEnv, envCfg, isRemote := p.cfg.GetActiveRemoteLogTarget()
	if !isRemote {
		return nil, fmt.Errorf("当前日志来源不是远端云日志 (dev/test/prod)，无法执行历史搜索")
	}

	rlCfg := p.cfg.GetRemoteLogConfig()
	token := strings.TrimSpace(rlCfg.AuthToken)
	if token == "" {
		return nil, fmt.Errorf("未配置远端日志 Authorization Token，请在设置中配置")
	}

	workflowURL := rlCfg.WorkflowURL
	if workflowURL == "" {

		return nil, fmt.Errorf("未配置远端日志查询地址 workflow_url，请在项目配置中提供")

	}

	environment := envCfg.GetEnvironment()

	service := envCfg.Service
	if service == "" {
		service = rlCfg.Service
	}
	if service == "" {
		service = rlCfg.Service
	}

	source := envCfg.Source
	if source == "" {
		source = rlCfg.Source
	}
	if source == "" {
		source = "default"
	}

	// History search window: custom [start, end] wins over preset time_range
	var startTime, endTime int64
	if params.StartTime > 0 || params.EndTime > 0 {
		endTime = params.EndTime
		if endTime <= 0 {
			endTime = time.Now().Unix()
		}
		startTime = params.StartTime
		if startTime <= 0 {
			startTime = endTime - int64((24 * time.Hour).Seconds())
		}
		if startTime >= endTime {
			return nil, fmt.Errorf("未配置远端日志查询地址 workflow_url，请在项目配置中提供")
		}
	} else {
		timeRange := strings.TrimSpace(params.TimeRange)
		window, ok := allowedSearchTimeRanges[timeRange]
		if !ok {
			return nil, fmt.Errorf("不支持的时间范围 %q，可选: 15m / 1h / 24h / 7d 或自定义时间范围", timeRange)
		}
		endTime = time.Now().Unix()
		startTime = endTime - int64(window.Seconds())
	}

	queryMode := strings.TrimSpace(params.QueryMode)
	if queryMode == "" {
		queryMode = "keyword"
	}
	if queryMode != "keyword" && queryMode != "sls" {
		return nil, fmt.Errorf("不支持的模式 %q，可选: keyword / sls", queryMode)
	}

	maxResults := params.MaxResults
	if maxResults <= 0 {
		maxResults = 10000
	}
	if maxResults > 10000 {
		maxResults = 10000
	}

	// Pause realtime polling during the search
	p.mu.Lock()
	if p.searching {
		p.mu.Unlock()
		return nil, fmt.Errorf("已有历史搜索在执行中，请稍候")
	}
	p.searching = true
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.searching = false
		p.mu.Unlock()
	}()

	// Fresh search window: clear hub (buffer + stats) and dedup registry.
	// Record the target env so the next realtime poll does not treat this as
	// an environment switch and wipe the freshly populated dedup registry.
	p.mu.Lock()
	p.lastTargetEnv = targetEnv
	p.mu.Unlock()
	p.hub.Clear()
	p.ResetWindow()

	const searchPageSize = 500
	var fetched, ingested, pages int
	truncated := false

	// Collect all pages first, then ingest in chronological order: the API
	// returns newest-first (both within a page and across pages), so the last
	// page holds the oldest entries and must be ingested first.
	allPages := make([][]RemoteLogItem, 0, 16)

	for page := 1; ; page++ {
		if fetched >= maxResults {
			truncated = true
			break
		}

		logs, err := p.fetchRemoteLogs(remoteQuery{
			workflowURL: workflowURL,
			token:       token,
			environment: environment,
			service:     service,
			source:      source,
			queryMode:   queryMode,
			startTime:   startTime,
			endTime:     endTime,
			keyword:     params.Keyword,
			page:        page,
			pageSize:    searchPageSize,
		})
		if err != nil {
			if pages == 0 {
				// First page failed: nothing ingested, surface the error
				return nil, err
			}
			// Later pages failed: keep what we already collected
			break
		}

		pages++
		fetched += len(logs)
		allPages = append(allPages, logs)

		// Last page reached (fewer items than requested) or hard page cap
		if len(logs) < searchPageSize || page >= 50 {
			if page >= 50 && len(logs) == searchPageSize {
				truncated = true
			}
			break
		}
	}

	// Ingest oldest page first; ingestRemoteItems reverses items within each
	// page (newest-first → chronological).
	for i := len(allPages) - 1; i >= 0; i-- {
		ing, _ := p.ingestRemoteItems(allPages[i], targetEnv, environment, time.Now())
		ingested += ing
	}

	p.mu.Lock()
	p.lastError = ""
	p.lastSuccessAt = time.Now()
	p.mu.Unlock()

	return &SearchResult{
		Success:    true,
		Message:    fmt.Sprintf("历史搜索完成，共拉取 %d 条日志", ingested),
		Fetched:    fetched,
		Ingested:   ingested,
		Pages:      pages,
		Truncated:  truncated,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}
