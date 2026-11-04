// Package web serves the read-only monitor UI and API over local HTTP. It
// depends on the monitor package for data and owns log-token signing and the
// bounded import store. It has no configuration or process-management duties.
package web

import (
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"local/sql-monitor/internal/analyze"
	"local/sql-monitor/internal/config"
	"local/sql-monitor/internal/devops"
	"local/sql-monitor/internal/monitor"
)

//go:embed index.html
var IndexHTML []byte

//go:embed logs.html
var LogsHTML []byte

//go:embed live.html
var LiveHTML []byte

var liveReadSlots = make(chan struct{}, 2)

var rawRoute = regexp.MustCompile(`^/api/event/([0-9]{1,20})\?raw=1$`)

const contentSecurityPolicy = "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

type Handler struct {
	monitor    *monitor.Monitor
	analyzer   *analyze.Analyzer
	logs       *logAccess
	remote     *devops.Poller
	configPath string
	port       int
}

func NewHandler(m *monitor.Monitor, port int) *Handler {
	return &Handler{monitor: m, logs: &logAccess{}, remote: devops.Disabled(), port: port}
}

// SetAnalyzer attaches the optional LLM summarizer. When nil (the default),
// the analyze endpoints report analysis as disabled.
func (h *Handler) SetAnalyzer(a *analyze.Analyzer) {
	h.analyzer = a
}

func (h *Handler) SetDevops(cfg config.DevopsConfig) {
	h.remote = devops.New(cfg)
}

func (h *Handler) SetConfigPath(path string) {
	h.configPath = path
}

// Close releases the import store's temporary directory and stops its sweeper.
func (h *Handler) Close() {
	h.logs.imports.close()
}

func (h Handler) trusted(r *http.Request) bool {
	host := strings.ToLower(r.Host)
	port := strconv.Itoa(h.port)
	if host != "127.0.0.1:"+port && host != "localhost:"+port && !(h.port == 80 && (host == "127.0.0.1" || host == "localhost")) {
		return false
	}
	if len(r.Header.Values("Host")) != 0 {
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) > 0 && (len(origins) != 1 || strings.ToLower(origins[0]) != "http://"+host) {
		return false
	}
	for _, site := range r.Header.Values("Sec-Fetch-Site") {
		if strings.EqualFold(site, "cross-site") {
			return false
		}
	}
	return true
}

func sendData(w http.ResponseWriter, r *http.Request, code int, data []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("Connection", "close")
	r.Close = true
	w.WriteHeader(code)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func sendJSON(w http.ResponseWriter, r *http.Request, code int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		code, data = http.StatusInternalServerError, []byte(`{"error":"Cannot encode response"}`)
	}
	sendData(w, r, code, data, "application/json; charset=utf-8")
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.trusted(r) {
		sendJSON(w, r, http.StatusForbidden, map[string]string{"error": "Untrusted host or origin"})
		return
	}
	if importEndpoint(r.RequestURI) {
		h.serveImport(w, r)
		return
	}
	if r.RequestURI == "/api/analyze" {
		h.serveAnalyze(w, r)
		return
	}
	if r.RequestURI == "/api/devops" {
		h.serveDevops(w, r)
		return
	}
	if r.RequestURI == "/api/feed" {
		h.serveFeed(w, r)
		return
	}
	if r.Method != http.MethodGet {
		sendJSON(w, r, http.StatusMethodNotAllowed, map[string]string{"error": "Read-only server; use GET"})
		return
	}
	switch r.RequestURI {
	case "/", "/overview", "/requests", "/live":
		sendData(w, r, http.StatusOK, LiveHTML, "text/html; charset=utf-8")
	case "/sql":
		sendData(w, r, http.StatusOK, IndexHTML, "text/html; charset=utf-8")
	case "/api/live?raw=1":
		h.serveLive(w, r, false)
	case "/api/live?raw=1&source=remote":
		h.serveLive(w, r, true)
	case "/api/status":
		sendJSON(w, r, http.StatusOK, h.data().StatusSnapshot())
	case "/api/state":
		sendJSON(w, r, http.StatusOK, h.data().Snapshot())
	case "/api/aggregates":
		sendJSON(w, r, http.StatusOK, h.data().AggregateSnapshot())
	case "/api/timeline":
		sendJSON(w, r, http.StatusOK, h.data().TimeSeriesSnapshot())
	case "/api/analyze/status":
		h.serveAnalyzeStatus(w, r)
	case "/logs":
		sendData(w, r, http.StatusOK, LogsHTML, "text/html; charset=utf-8")
	default:
		if strings.HasPrefix(r.RequestURI, "/api/logs/") && r.URL.EscapedPath() == r.URL.Path {
			h.serveLogs(w, r)
			return
		}
		match := rawRoute.FindStringSubmatch(r.RequestURI)
		if match == nil {
			sendJSON(w, r, http.StatusNotFound, map[string]string{"error": "Not found"})
			return
		}
		id, err := strconv.ParseUint(match[1], 10, 64)
		if err == nil {
			if raw, found := h.data().RawEvent(id); found {
				sendJSON(w, r, http.StatusOK, raw)
				return
			}
		}
		sendJSON(w, r, http.StatusNotFound, map[string]string{"error": "Event not found"})
	}
}

// serveAnalyze triggers a manual analysis run over the cumulative hotspots. The
// request body is drained and ignored; it exists only to keep clients free to
// send a no-op payload. Analysis is at-most-one-at-a-time: a trigger while a run
// is in flight returns 409 with the current status.
func (h Handler) serveDevops(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		tokenSet, environment, service := h.remote.Settings()
		sendJSON(w, r, http.StatusOK, map[string]any{
			"token_set":   tokenSet,
			"environment": environment,
			"service":     service,
			"environments": []map[string]string{
				{"id": "dev", "namespace": "ns-cloud-dev"},
				{"id": "test", "namespace": "ns-cloud-test"},
				{"id": "prod", "namespace": "ns-cloud-prod"},
			},
		})
		return
	}
	if r.Method != http.MethodPost {
		sendJSON(w, r, http.StatusMethodNotAllowed, map[string]string{"error": "Use GET or POST"})
		return
	}
	origins := r.Header.Values("Origin")
	if len(origins) != 1 || origins[0] != "http://"+r.Host {
		sendJSON(w, r, http.StatusForbidden, map[string]string{"error": "Untrusted origin"})
		return
	}
	var body struct {
		Token       string `json:"token"`
		Environment string `json:"environment"`
		Service     string `json:"service"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		sendJSON(w, r, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}
	if err := h.remote.Apply(config.DevopsConfig{Token: body.Token, Environment: body.Environment, Service: body.Service}); err != nil {
		sendJSON(w, r, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}
	saved := false
	if h.configPath != "" {
		if err := config.SaveDevops(h.configPath, h.remote.Export()); err == nil {
			saved = true
		}
	}
	_, environment, service := h.remote.Settings()
	sendJSON(w, r, http.StatusOK, map[string]any{"ok": true, "saved": saved, "token_set": true, "environment": environment, "service": service})
}

func (h Handler) data() *monitor.Monitor {
	if h.remote != nil && h.remote.Active() {
		if logs := h.remote.Logs(); logs != nil {
			return logs
		}
	}
	return h.monitor
}

func (h Handler) serveFeed(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		source := "local"
		if h.remote != nil && h.remote.Active() {
			source = "remote"
		}
		sendJSON(w, r, http.StatusOK, map[string]string{"source": source})
	case http.MethodPost:
		origins := r.Header.Values("Origin")
		if len(origins) != 1 || origins[0] != "http://"+r.Host {
			sendJSON(w, r, http.StatusForbidden, map[string]string{"error": "Untrusted origin"})
			return
		}
		var body struct {
			Source string `json:"source"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil || (body.Source != "local" && body.Source != "remote") {
			sendJSON(w, r, http.StatusBadRequest, map[string]string{"error": "source must be local or remote"})
			return
		}
		if h.remote != nil {
			h.remote.SetActive(body.Source == "remote")
		}
		sendJSON(w, r, http.StatusOK, map[string]string{"source": body.Source})
	default:
		sendJSON(w, r, http.StatusMethodNotAllowed, map[string]string{"error": "Use GET or POST"})
	}
}

func (h Handler) serveLive(w http.ResponseWriter, r *http.Request, remote bool) {
	select {
	case liveReadSlots <- struct{}{}:
		defer func() { <-liveReadSlots }()
		if remote || (h.remote != nil && h.remote.Active()) {
			sendJSON(w, r, http.StatusOK, h.remote.View())
			return
		}
		sendJSON(w, r, http.StatusOK, h.monitor.LiveSnapshot())
	default:
		sendJSON(w, r, http.StatusTooManyRequests, map[string]string{"error": "Too many live snapshot readers"})
	}
}

func (h Handler) serveAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendJSON(w, r, http.StatusMethodNotAllowed, map[string]string{"error": "Use POST to trigger analysis"})
		return
	}
	if h.analyzer == nil {
		sendJSON(w, r, http.StatusNotFound, map[string]string{"error": "Analysis is not enabled"})
		return
	}
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 4096))
	}
	started := h.analyzer.TriggerAggregates(h.data().AggregateSnapshot().Groups)
	status := h.analyzer.Status()
	if !started {
		if status.Running {
			sendJSON(w, r, http.StatusConflict, map[string]any{"started": false, "status": status})
		} else {
			sendJSON(w, r, http.StatusOK, map[string]any{"started": false, "reason": "no_hotspots", "status": status})
		}
		return
	}
	sendJSON(w, r, http.StatusAccepted, map[string]any{"started": true, "status": status})
}

func (h Handler) serveAnalyzeStatus(w http.ResponseWriter, r *http.Request) {
	if h.analyzer == nil {
		sendJSON(w, r, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	sendJSON(w, r, http.StatusOK, map[string]any{"enabled": true, "status": h.analyzer.Status()})
}
