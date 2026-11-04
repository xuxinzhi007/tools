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
	staticFS     embed.FS
	httpServer   *http.Server
}

// NewServer creates a new Server
func NewServer(
	cfg *config.Config,
	parser *openapi.Parser,
	proxyClient *proxy.Proxy,
	hub *logger.Hub,
	tailer *logger.FileTailer,
	runner *logger.ProcessRunner,
	remotePoller *logger.RemoteLogPoller,
	staticFS embed.FS,
) *Server {
	s := &Server{
		cfg:          cfg,
		parser:       parser,
		proxy:        proxyClient,
		hub:          hub,
		tailer:       tailer,
		runner:       runner,
		remotePoller: remotePoller,
		staticFS:     staticFS,
	}

	// Initial load of OpenAPI spec
	spec, err := parser.ParseFromFile(cfg.OpenAPIPath)
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
	if cmd == "" {
		cmd = "./bin/data-service"
	}
	args := body.Args
	if len(args) == 0 {
		args = []string{"-conf", "configs"}
	}
	dir := body.Dir
	if dir == "" {
		dir = "../data-service"
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
