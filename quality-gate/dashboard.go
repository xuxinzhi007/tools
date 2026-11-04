package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxEventLog     = 200
	serverVersion   = "1.4.0"
	dashboardAPIVer = "1"
)

type toolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema gin.H  `json:"inputSchema"`
}

type toolStats struct {
	CallCount    int64  `json:"call_count"`
	ErrorCount   int64  `json:"error_count"`
	LastCalledAt string `json:"last_called_at,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

type dashboardEvent struct {
	Time       string `json:"time"`
	Type       string `json:"type"`
	Method     string `json:"method,omitempty"`
	Tool       string `json:"tool,omitempty"`
	Transport  string `json:"transport"`
	ClientIP   string `json:"client_ip"`
	Path       string `json:"path"`
	Success    bool   `json:"success"`
	DurationMS int64  `json:"duration_ms"`
	Detail     string `json:"detail,omitempty"`
}

type dashboardState struct {
	mu              sync.RWMutex
	startedAt       time.Time
	totalRequests   int64
	toolCalls       int64
	errors          int64
	sessionsCreated int64
	toolStats       map[string]*toolStats
	events          []dashboardEvent
	sseConnections  int64
	mcpStreams      int64
}

var dash dashboardState

func initDashboard() {
	dash = dashboardState{
		startedAt: time.Now(),
		toolStats: make(map[string]*toolStats),
		events:    make([]dashboardEvent, 0, 64),
	}
	for _, t := range toolDefinitions() {
		dash.toolStats[t.Name] = &toolStats{}
	}
}

type mcpCallContext struct {
	clientIP  string
	transport string
	path      string
	started   time.Time
}

func newMCPContext(c *gin.Context, transport string) mcpCallContext {
	return mcpCallContext{
		clientIP:  c.ClientIP(),
		transport: transport,
		path:      c.Request.URL.Path,
		started:   time.Now(),
	}
}

func recordMCPRequest(ctx mcpCallContext, method string, success bool, detail string) {
	dash.mu.Lock()
	defer dash.mu.Unlock()

	dash.totalRequests++
	if !success {
		dash.errors++
	}

	evt := dashboardEvent{
		Time:       time.Now().Format(time.RFC3339),
		Type:       "request",
		Method:     method,
		Transport:  ctx.transport,
		ClientIP:   ctx.clientIP,
		Path:       ctx.path,
		Success:    success,
		DurationMS: time.Since(ctx.started).Milliseconds(),
		Detail:     detail,
	}
	dash.pushEvent(evt)
}

func recordSessionCreated(ctx mcpCallContext) {
	dash.mu.Lock()
	defer dash.mu.Unlock()

	dash.sessionsCreated++
	dash.pushEvent(dashboardEvent{
		Time:       time.Now().Format(time.RFC3339),
		Type:       "session",
		Method:     "initialize",
		Transport:  ctx.transport,
		ClientIP:   ctx.clientIP,
		Path:       ctx.path,
		Success:    true,
		DurationMS: time.Since(ctx.started).Milliseconds(),
		Detail:     "MCP session initialized",
	})
}

func recordToolCall(ctx mcpCallContext, tool string, success bool, detail string, errMsg string) {
	dash.mu.Lock()
	defer dash.mu.Unlock()

	dash.totalRequests++
	dash.toolCalls++
	if !success {
		dash.errors++
	}

	stats, ok := dash.toolStats[tool]
	if !ok {
		stats = &toolStats{}
		dash.toolStats[tool] = stats
	}
	stats.CallCount++
	stats.LastCalledAt = time.Now().Format(time.RFC3339)
	if !success {
		stats.ErrorCount++
		stats.LastError = errMsg
	}

	dash.pushEvent(dashboardEvent{
		Time:       time.Now().Format(time.RFC3339),
		Type:       "tool_call",
		Method:     "tools/call",
		Tool:       tool,
		Transport:  ctx.transport,
		ClientIP:   ctx.clientIP,
		Path:       ctx.path,
		Success:    success,
		DurationMS: time.Since(ctx.started).Milliseconds(),
		Detail:     detail,
	})
}

func (d *dashboardState) pushEvent(evt dashboardEvent) {
	d.events = append(d.events, evt)
	if len(d.events) > maxEventLog {
		d.events = d.events[len(d.events)-maxEventLog:]
	}
}

func incSSEConnections(delta int64) {
	dash.mu.Lock()
	dash.sseConnections += delta
	dash.mu.Unlock()
}

func incMCPStreams(delta int64) {
	dash.mu.Lock()
	dash.mcpStreams += delta
	dash.mu.Unlock()
}

func countActiveSessions() int {
	n := 0
	sessions.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

func countSSESessions() int {
	n := 0
	sseSessions.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

func dashboardSnapshot() gin.H {
	dash.mu.RLock()
	defer dash.mu.RUnlock()

	tools := make([]gin.H, 0, len(toolDefinitions()))
	for _, def := range toolDefinitions() {
		stats := dash.toolStats[def.Name]
		if stats == nil {
			stats = &toolStats{}
		}
		tools = append(tools, gin.H{
			"name":           def.Name,
			"description":    def.Description,
			"input_schema":   def.InputSchema,
			"call_count":     stats.CallCount,
			"error_count":    stats.ErrorCount,
			"last_called_at": stats.LastCalledAt,
			"last_error":     stats.LastError,
		})
	}

	events := make([]dashboardEvent, len(dash.events))
	copy(events, dash.events)
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	tunnel := currentTunnelInfo()

	return gin.H{
		"api_version": dashboardAPIVer,
		"server": ginH{
			"name":             "go-file-mcp",
			"version":          serverVersion,
			"protocol":         protocolVersion,
			"started_at":       dash.startedAt.Format(time.RFC3339),
			"uptime_seconds":   int64(time.Since(dash.startedAt).Seconds()),
			"port":             appConfig.serverPort(),
			"root":             baseDir,
			"auth_enabled":     authToken != "",
			"config_path":      envOr("CONFIG_PATH", "config.json"),
			"ngrok_auto_start": appConfig.Ngrok.AutoStart,
			"status":           "online",
		},
		"tunnel": tunnel,
		"sessions": ginH{
			"active_mcp":    countActiveSessions(),
			"active_sse":    countSSESessions(),
			"total_created": dash.sessionsCreated,
			"live_streams":  dash.mcpStreams,
		},
		"stats": ginH{
			"total_requests": dash.totalRequests,
			"tool_calls":     dash.toolCalls,
			"errors":         dash.errors,
		},
		"tools": tools,
		"endpoints": []ginH{
			{"method": "POST", "path": "/mcp", "transport": "streamable-http", "description": "MCP JSON-RPC (recommended)"},
			{"method": "GET", "path": "/mcp", "transport": "streamable-http", "description": "MCP SSE stream"},
			{"method": "DELETE", "path": "/mcp", "transport": "streamable-http", "description": "Close MCP session"},
			{"method": "GET", "path": "/sse", "transport": "legacy-sse", "description": "Legacy SSE handshake"},
			{"method": "POST", "path": "/sse", "transport": "legacy-sse", "description": "Legacy SSE JSON-RPC"},
			{"method": "POST", "path": "/messages", "transport": "legacy-sse", "description": "Legacy SSE messages"},
			{"method": "GET", "path": "/api/dashboard", "transport": "http", "description": "Dashboard snapshot JSON"},
			{"method": "GET", "path": "/api/dashboard/stream", "transport": "sse", "description": "Dashboard live SSE"},
		},
		"recent_events": events,
		"updated_at":    time.Now().Format(time.RFC3339),
	}
}

func registerDashboardRoutes(r *gin.Engine) {
	r.GET("/api/dashboard", func(c *gin.Context) {
		c.JSON(http.StatusOK, dashboardSnapshot())
	})

	r.GET("/api/dashboard/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		send := func() {
			data, err := json.Marshal(dashboardSnapshot())
			if err != nil {
				return
			}
			fmt.Fprintf(c.Writer, "event: snapshot\ndata: %s\n\n", data)
			flusher.Flush()
		}

		send()
		for {
			select {
			case <-c.Request.Context().Done():
				return
			case <-ticker.C:
				send()
			}
		}
	})
}
