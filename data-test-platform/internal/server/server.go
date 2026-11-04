package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"data-test-platform/internal/config"
	"data-test-platform/internal/logger"
	"data-test-platform/internal/openapi"
	"data-test-platform/internal/proxy"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local test tool
	},
}

// Server provides HTTP and WebSocket APIs
type Server struct {
	cfg          *config.Config
	parser       *openapi.Parser
	spec         *openapi.ParsedSpec
	specMu       sync.RWMutex
	proxy        *proxy.Proxy
	hub          *logger.Hub
	tailer       *logger.FileTailer
	runner       *logger.ProcessRunner
	remotePoller *logger.RemoteLogPoller
	k8sTailer    *logger.K8sTailer
	staticFS     embed.FS
	httpServer   *http.Server
}

// Deps 汇集 Server 的运行时依赖，避免构造参数过长。
type Deps struct {
	Config       *config.Config
	Parser       *openapi.Parser
	Proxy        *proxy.Proxy
	Hub          *logger.Hub
	Tailer       *logger.FileTailer
	Runner       *logger.ProcessRunner
	RemotePoller *logger.RemoteLogPoller
	K8sTailer    *logger.K8sTailer
	StaticFS     embed.FS
}

// NewServer creates a new Server
func NewServer(d Deps) *Server {
	cfg := d.Config
	s := &Server{
		cfg:          cfg,
		parser:       d.Parser,
		proxy:        d.Proxy,
		hub:          d.Hub,
		tailer:       d.Tailer,
		runner:       d.Runner,
		remotePoller: d.RemotePoller,
		k8sTailer:    d.K8sTailer,
		staticFS:     d.StaticFS,
	}

	// Initial load of OpenAPI spec
	spec, err := d.Parser.ParseFromFile(cfg.OpenAPIPath)
	if err != nil {
		fmt.Printf("⚠️ Warning: Could not load OpenAPI file '%s': %v\n", cfg.OpenAPIPath, err)
		fmt.Println("ℹ️ You can reload or upload an OpenAPI file later via the UI.")
		s.spec = &openapi.ParsedSpec{
			Title:      "Data Service API",
			TotalCount: 0,
			Groups:     []openapi.TagGroup{},
			Endpoints:  []*openapi.Operation{},
		}
	} else {
		s.spec = spec
		fmt.Printf("✅ Successfully loaded OpenAPI: %d endpoints across %d services\n", spec.TotalCount, len(spec.Tags))
	}

	return s
}

// Routes sets up the HTTP router
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// OpenAPI APIs
	mux.HandleFunc("/api/openapi", s.handleGetOpenAPI)
	mux.HandleFunc("/api/openapi/reload", s.handleReloadOpenAPI)
	mux.HandleFunc("/api/openapi/upload", s.handleUploadOpenAPI)

	// Proxy API
	mux.HandleFunc("/api/proxy", s.handleProxy)

	// Log & SQL Monitor APIs
	mux.HandleFunc("/api/logs/recent", s.handleGetRecentLogs)
	mux.HandleFunc("/api/logs/ingest", s.handleIngestLogs)
	mux.HandleFunc("/api/logs/mock", s.handleMockLogs)
	mux.HandleFunc("/api/logs/stats", s.handleGetStats)
	mux.HandleFunc("/api/logs/clear", s.handleClearLogs)
	mux.HandleFunc("/api/logs/aggregates", s.handleGetAggregates)
	mux.HandleFunc("/ws/logs", s.handleWebSocketLogs)

	// Remote Cloud Log APIs
	mux.HandleFunc("/api/logs/remote/status", s.handleRemoteLogStatus)
	mux.HandleFunc("/api/logs/remote/test", s.handleRemoteLogTest)
	mux.HandleFunc("/api/logs/remote/search", s.handleRemoteLogSearch)

	// K8s Log Source APIs（可选，未启用时只返回配置状态）
	mux.HandleFunc("/api/logs/k8s/status", s.handleK8sStatus)
	mux.HandleFunc("/api/logs/k8s/start", s.handleK8sStart)
	mux.HandleFunc("/api/logs/k8s/stop", s.handleK8sStop)

	// Config APIs
	mux.HandleFunc("/api/config", s.handleConfig)

	// Process APIs
	mux.HandleFunc("/api/process/status", s.handleProcessStatus)
	mux.HandleFunc("/api/process/start", s.handleProcessStart)
	mux.HandleFunc("/api/process/stop", s.handleProcessStop)

	// Static Web Assets
	distFS, err := fs.Sub(s.staticFS, "web/dist")
	if err == nil {
		fileServer := http.FileServer(http.FS(distFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
				http.NotFound(w, r)
				return
			}
			// If file doesn't exist, serve index.html for SPA routing
			f, err := distFS.Open(strings.TrimPrefix(r.URL.Path, "/"))
			if err != nil {
				r.URL.Path = "/"
			} else {
				_ = f.Close()
			}
			fileServer.ServeHTTP(w, r)
		})
	} else {
		// Fallback local directory serving if embed.FS is empty
		mux.Handle("/", http.FileServer(http.Dir("web/dist")))
	}

	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-tenant-id, x-user-id, x-trace-id")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) handleGetOpenAPI(w http.ResponseWriter, r *http.Request) {
	s.specMu.RLock()
	defer s.specMu.RUnlock()
	writeJSON(w, http.StatusOK, s.spec)
}

func (s *Server) handleReloadOpenAPI(w http.ResponseWriter, r *http.Request) {
	filePath := s.cfg.OpenAPIPath
	spec, err := s.parser.ParseFromFile(filePath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("Failed to reload '%s': %v", filePath, err),
		})
		return
	}

	s.specMu.Lock()
	s.spec = spec
	s.specMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":     fmt.Sprintf("Reloaded successfully: %d endpoints", spec.TotalCount),
		"total_count": spec.TotalCount,
		"tags":        len(spec.Tags),
	})
}

func (s *Server) handleUploadOpenAPI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid form data"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing file in upload"})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read upload"})
		return
	}

	spec, err := s.parser.ParseBytes(content, time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Failed to parse OpenAPI: %v", err)})
		return
	}

	s.specMu.Lock()
	s.spec = spec
	s.specMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":     fmt.Sprintf("Uploaded '%s' successfully: %d endpoints", header.Filename, spec.TotalCount),
		"total_count": spec.TotalCount,
		"tags":        len(spec.Tags),
	})
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var reqPayload proxy.RequestPayload
	if err := json.NewDecoder(r.Body).Decode(&reqPayload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	// If relative path, prefix with current environment baseURL
	if strings.HasPrefix(reqPayload.URL, "/") {
		baseURL := strings.TrimRight(s.cfg.GetBaseURL(), "/")
		reqPayload.URL = baseURL + reqPayload.URL
	}

	resp := s.proxy.Execute(&reqPayload)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetRecentLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 500
	}
	level := q.Get("level")
	search := q.Get("search")
	sqlOnly := q.Get("sql_only") == "true"
	slowOnly := q.Get("slow_only") == "true"

	logs := s.hub.GetRecentLogs(limit, level, search, sqlOnly, slowOnly)
	stats := s.hub.Stats().GetStats()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs":  logs,
		"stats": stats,
		"count": len(logs),
	})
}

func (s *Server) handleIngestLogs(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read body"})
		return
	}

	lines := strings.Split(string(body), "\n")
	ingested := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			s.hub.IngestRawLine(trimmed)
			ingested++
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"ingested": ingested,
	})
}

func (s *Server) handleMockLogs(w http.ResponseWriter, r *http.Request) {
	count := 15
	if cStr := r.URL.Query().Get("count"); cStr != "" {
		if c, err := strconv.Atoi(cStr); err == nil && c > 0 {
			count = c
		}
	}
	go logger.GenerateMockLogs(s.hub, count)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("Generating %d simulated logs & SQL queries in background", count),
	})
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := s.hub.Stats().GetStats()
	writeJSON(w, http.StatusOK, stats)
}

// handleGetAggregates returns cumulative SQL fingerprint groups. Unlike the
// retained log buffer, these counts survive eviction and cover the whole
// process lifetime. ?sort=hotspot ranks by weighted anomaly score.
func (s *Server) handleGetAggregates(w http.ResponseWriter, r *http.Request) {
	store := s.hub.Aggregates()
	groups := store.Snapshot()

	if r.URL.Query().Get("sort") == "hotspot" {
		groups = logger.RankHotspots(groups)
	}

	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	total := len(groups)
	if limit > 0 && len(groups) > limit {
		groups = groups[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"groups":   groups,
		"total":    total,
		"returned": len(groups),
		"overflow": store.Overflow(),
	})
}

func (s *Server) handleClearLogs(w http.ResponseWriter, r *http.Request) {
	s.hub.Clear()
	writeJSON(w, http.StatusOK, map[string]string{"message": "Logs and stats cleared"})
}

func (s *Server) handleWebSocketLogs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("WebSocket upgrade failed: %v\n", err)
		return
	}
	s.hub.RegisterClient(conn)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"current_env_id":       s.cfg.CurrentEnvID,
			"current_base_url":     s.cfg.GetBaseURL(),
			"current_route_prefix": s.cfg.GetRoutePrefix(),
			"custom_base_url":      s.cfg.CustomBaseURL,
			"environments":         s.cfg.Environments,
			"slow_sql_threshold":   s.cfg.GetSlowSQLThreshold().String(),
			"openapi_path":         s.cfg.OpenAPIPath,
			"log_file_path":        s.cfg.LogFilePath,
			"default_tenant_id":    s.cfg.DefaultTenantID,
			"default_user_id":      s.cfg.DefaultUserID,
			"default_token":        s.cfg.DefaultToken,
			"enable_xr_auth":       s.cfg.EnableXrAuth,
			"xr_auth_type":         s.cfg.XrAuthType,
			"xr_auth_uid":          s.cfg.XrAuthUID,
			"xr_auth_sub":          s.cfg.XrAuthSub,
			"xr_auth_tid":          s.cfg.XrAuthTID,
			"xr_auth_aut":          s.cfg.XrAuthAut,
			"remote_log":           s.cfg.GetRemoteLogConfig(),
			"remote_log_status":    s.remotePoller.GetStatus(),

			// 项目 profile：前端根据这些渲染标题、环境和默认请求头，
			// 不再在 JS 里写死任何域名或账号。
			"project_name":    s.cfg.ProjectName,
			"project_title":   s.cfg.ProjectTitle,
			"profile_headers": s.resolvedHeaders(),
		})
		return
	}

	if r.Method == "POST" || r.Method == "PUT" {
		var body struct {
			CurrentEnvID     string                  `json:"current_env_id"`
			CurrentPrefix    string                  `json:"current_route_prefix"`
			RoutePrefix      string                  `json:"route_prefix"`
			CustomBaseURL    string                  `json:"custom_base_url"`
			SlowSQLThreshold string                  `json:"slow_sql_threshold"`
			LogFilePath      string                  `json:"log_file_path"`
			DefaultTenantID  string                  `json:"default_tenant_id"`
			DefaultUserID    string                  `json:"default_user_id"`
			DefaultToken     string                  `json:"default_token"`
			EnableXrAuth     *bool                   `json:"enable_xr_auth"`
			XrAuthType       string                  `json:"xr_auth_type"`
			XrAuthUID        string                  `json:"xr_auth_uid"`
			XrAuthSub        string                  `json:"xr_auth_sub"`
			XrAuthTID        string                  `json:"xr_auth_tid"`
			XrAuthAut        string                  `json:"xr_auth_aut"`
			RemoteLog        *config.RemoteLogConfig `json:"remote_log"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid body"})
			return
		}

		prefix := body.RoutePrefix
		if prefix == "" {
			prefix = body.CurrentPrefix
		}

		// 记录更新前的有效日志来源,用于切换后清空日志流,避免新旧来源混杂
		sourceBefore, _, _ := s.cfg.GetActiveRemoteLogTarget()

		if body.CurrentEnvID != "" {
			s.cfg.SetCurrentEnv(body.CurrentEnvID, body.CustomBaseURL, prefix)
			s.remotePoller.ResetWindow()
		} else if prefix != "" {
			s.cfg.CurrentRoutePrefix = prefix
		}
		if body.SlowSQLThreshold != "" {
			if d, err := time.ParseDuration(body.SlowSQLThreshold); err == nil {
				s.cfg.SetSlowSQLThreshold(d)
				s.hub.Parser().SetSlowThreshold(d)
			}
		}
		if body.LogFilePath != "" {
			s.cfg.LogFilePath = body.LogFilePath
			s.tailer.SetFilePath(body.LogFilePath)
		}
		if body.DefaultTenantID != "" {
			s.cfg.DefaultTenantID = body.DefaultTenantID
		}
		if body.DefaultUserID != "" {
			s.cfg.DefaultUserID = body.DefaultUserID
		}
		if body.DefaultToken != "" {
			s.cfg.DefaultToken = body.DefaultToken
		}
		if body.EnableXrAuth != nil {
			s.cfg.EnableXrAuth = *body.EnableXrAuth
		}
		if body.XrAuthType != "" {
			s.cfg.XrAuthType = body.XrAuthType
		}
		if body.XrAuthUID != "" {
			s.cfg.XrAuthUID = body.XrAuthUID
		}
		if body.XrAuthSub != "" {
			s.cfg.XrAuthSub = body.XrAuthSub
		}
		if body.XrAuthTID != "" {
			s.cfg.XrAuthTID = body.XrAuthTID
		}
		if body.XrAuthAut != "" {
			s.cfg.XrAuthAut = body.XrAuthAut
		}
		if body.RemoteLog != nil {
			s.cfg.UpdateRemoteLogConfig(*body.RemoteLog)
			s.remotePoller.ResetWindow()
		}

		// 日志来源发生变化时 (显式切换 log_source 或 auto 模式跟随环境变化),
		// 清空日志缓冲与 SQL 统计,使新来源从干净状态开始
		if sourceAfter, _, _ := s.cfg.GetActiveRemoteLogTarget(); sourceAfter != sourceBefore {
			s.hub.Clear()
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func (s *Server) handleRemoteLogStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.remotePoller.GetStatus())
}

// k8sStatusReply 返回 K8s 日志源的可选环境与当前运行状态。
// enabled 为 false 时前端只展示“未配置”，不会自动启动 kubectl。
type k8sStatusReply struct {
	Enabled      bool                  `json:"enabled"`
	Running      bool                  `json:"running"`
	State        string                `json:"state"`
	Describe     string                `json:"describe,omitempty"`
	Error        string                `json:"error,omitempty"`
	Environments []config.K8sEnvConfig `json:"environments"`
}

func (s *Server) handleK8sStatus(w http.ResponseWriter, r *http.Request) {
	reply := k8sStatusReply{
		Enabled:      s.cfg.K8s.Enabled,
		Environments: s.cfg.K8s.Environments,
		State:        "idle",
	}
	if s.k8sTailer != nil {
		running, state, describe, errMsg := s.k8sTailer.Status()
		reply.Running = running
		reply.State = state
		reply.Describe = describe
		reply.Error = errMsg
	}
	writeJSON(w, http.StatusOK, reply)
}

func (s *Server) handleK8sStart(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.K8s.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "K8s 日志源未启用。在配置里设 k8s.enabled=true 并填好 kubeconfig。",
		})
		return
	}
	if s.k8sTailer == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "k8s tailer 未初始化"})
		return
	}

	var body struct {
		Environment string `json:"environment"` // 匹配 cfg.K8s.Environments 里的 Name
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	env := config.K8sEnvConfig{}
	for _, e := range s.cfg.K8s.Environments {
		if e.Name == body.Environment {
			env = e
			break
		}
	}
	if body.Environment != "" && env.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "未知环境: " + body.Environment,
		})
		return
	}

	// 切到 K8s 前先停掉本地与云端源，避免多条流混在一起
	s.tailer.Stop()
	s.remotePoller.Stop()

	s.k8sTailer.Start(logger.K8sTarget{
		Environment: env.Name,
		Kubectl:     s.cfg.K8s.Kubectl,
		Kubeconfig:  s.cfg.K8s.Kubeconfig,
		Context:     s.cfg.K8s.Context,
		Namespace:   env.Namespace,
		App:         s.cfg.K8s.App,
		AppLabel:    s.cfg.K8s.AppLabel,
		LaneLabel:   s.cfg.K8s.LaneLabel,
		Lane:        env.Lane,
		Container:   s.cfg.K8s.Container,
		Tail:        s.cfg.K8s.Tail,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "environment": env.Name})
}

func (s *Server) handleK8sStop(w http.ResponseWriter, r *http.Request) {
	if s.k8sTailer != nil {
		s.k8sTailer.Stop()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRemoteLogTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
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
	_ = json.NewDecoder(r.Body).Decode(&body)

	rlCfg := s.cfg.GetRemoteLogConfig()
	_, envCfg, _ := s.cfg.GetActiveRemoteLogTarget()

	if body.WorkflowURL == "" {
		body.WorkflowURL = rlCfg.WorkflowURL
	}
	if body.AuthToken == "" {
		body.AuthToken = rlCfg.AuthToken
	}
	if body.Environment == "" {
		body.Environment = body.Namespace
	}
	if body.Environment == "" {
		body.Environment = envCfg.GetEnvironment()
	}
	if body.Service == "" {
		body.Service = envCfg.Service
		if body.Service == "" {
			body.Service = rlCfg.Service
		}
	}
	if body.Source == "" {
		body.Source = envCfg.Source
		if body.Source == "" {
			body.Source = rlCfg.Source
		}
	}

	res, err := s.remotePoller.TestConnection(logger.TestConnectionParams{
		WorkflowURL: body.WorkflowURL,
		AuthToken:   body.AuthToken,
		Environment: body.Environment,
		Namespace:   body.Namespace,
		Service:     body.Service,
		Source:      body.Source,
		QueryMode:   body.QueryMode,
		TimeRange:   body.TimeRange,
		SLSProject:  body.SLSProject,
		ProjectIDs:  body.ProjectIDs,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRemoteLogSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		TimeRange  string `json:"time_range"`
		QueryMode  string `json:"query_mode"`
		Keyword    string `json:"keyword"`
		StartTime  int64  `json:"start_time"` // custom window start (unix seconds)
		EndTime    int64  `json:"end_time"`   // custom window end (unix seconds)
		MaxResults int    `json:"max_results"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "请求体解析失败: " + err.Error(),
		})
		return
	}

	res, err := s.remotePoller.SearchHistory(logger.SearchParams{
		TimeRange:  body.TimeRange,
		QueryMode:  body.QueryMode,
		Keyword:    body.Keyword,
		StartTime:  body.StartTime,
		EndTime:    body.EndTime,
		MaxResults: body.MaxResults,
	})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleProcessStatus(w http.ResponseWriter, r *http.Request) {
	running := s.runner.IsRunning()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"running": running,
	})
}

func (s *Server) handleProcessStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Dir     string   `json:"dir"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	cmd := body.Command
	args := body.Args
	dir := body.Dir
	// 默认启动命令来自 profile；未配置时不猜，免得拉起别的服务
	if s.cfg.Run != nil {
		if cmd == "" {
			cmd = s.cfg.Run.Command
		}
		if len(args) == 0 {
			args = s.cfg.Run.Args
		}
		if dir == "" {
			dir = s.cfg.Run.Dir
		}
	}
	if cmd == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "未指定启动命令，也没有在项目配置里提供 run.command",
		})
		return
	}
	if len(args) == 0 {
		args = []string{}
	}

	if _, err := os.Stat(dir + "/" + cmd); os.IsNotExist(err) && !strings.Contains(cmd, "/") {
		// check if binary exists
	}

	if err := s.runner.Start(cmd, args, dir); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) handleProcessStop(w http.ResponseWriter, r *http.Request) {
	if err := s.runner.Stop(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// Start launches the HTTP server
func (s *Server) Start(addr string) error {
	s.tailer.Start()
	s.remotePoller.Start()
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.Routes(),
	}
	return s.httpServer.ListenAndServe()
}

// resolvedHeaders 展开 profile 里的 ${var} 占位符。
// Secret 的规则不回显真实值，只给掩码，避免密钥从 /api/config 泄漏。
func (s *Server) resolvedHeaders() []config.HeaderRule {
	rules := make([]config.HeaderRule, 0, len(s.cfg.ProfileHeaders))
	profile := &config.ProjectProfile{Vars: s.cfg.ProfileVars}
	for _, r := range s.cfg.ProfileHeaders {
		out := r
		out.Value = profile.ExpandVars(r.Value)
		if r.Secret && out.Value != "" {
			out.Value = maskSecret(out.Value)
		}
		rules = append(rules, out)
	}
	return rules
}

func maskSecret(v string) string {
	if len(v) <= 6 {
		return "***"
	}
	return v[:3] + "***" + v[len(v)-3:]
}
