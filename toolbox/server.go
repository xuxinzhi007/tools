package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type toolView struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Desc       string   `json:"desc"`
	Group      string   `json:"group"`
	State      string   `json:"state"`
	Port       int      `json:"port"`
	URL        string   `json:"url"`
	PID        int      `json:"pid"`
	Err        string   `json:"err,omitempty"`
	Uptime     string   `json:"uptime,omitempty"`
	CanStart   bool     `json:"can_start"`
	Deprecated bool     `json:"deprecated,omitempty"`
	Transport  string   `json:"transport,omitempty"`
	Missing    []string `json:"missing,omitempty"`
	Dir        string   `json:"dir"`
	Source     string   `json:"source,omitempty"`
	Endpoint   string   `json:"endpoint,omitempty"`
}

func (s *server) handleAPI(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/tools" && r.Method == "GET":
		s.writeTools(w)
	case r.URL.Path == "/api/logs" && r.Method == "GET":
		id := r.URL.Query().Get("id")
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if n == 0 {
			n = 120
		}
		writeJSON(w, 200, map[string]any{"id": id, "lines": s.mgr.tail(id, n)})
	case r.URL.Path == "/api/start" && r.Method == "POST":
		id := r.URL.Query().Get("id")
		if err := s.mgr.start(id); err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case r.URL.Path == "/api/stop" && r.Method == "POST":
		id := r.URL.Query().Get("id")
		if err := s.mgr.stop(id); err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case r.URL.Path == "/api/reload" && r.Method == "POST":
		writeJSON(w, 200, map[string]any{"ok": true})
	case r.URL.Path == "/api/kill" && r.Method == "POST":
		id := r.URL.Query().Get("id")
		if err := s.mgr.killExternal(id); err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func (s *server) writeTools(w http.ResponseWriter) {
	out := make([]toolView, 0)
	for _, p := range s.mgr.all() {
		st := s.mgr.probe(p)
		p.mu.Lock()
		v := toolView{
			ID:         p.Tool.ID,
			Name:       p.Tool.Name,
			Desc:       p.Tool.Desc,
			Group:      p.Tool.Group,
			State:      string(st),
			Port:       p.Tool.Port,
			URL:        s.cfg.toolURL(p.Tool),
			PID:        p.PID,
			Err:        p.Err,
			CanStart:   len(p.Tool.Cmd) > 0,
			Deprecated: p.Tool.Deprecated,
			Transport:  p.Tool.Transport,
			Missing:    s.cfg.missing[p.Tool.ID],
			Dir:        s.cfg.toolDir(p.Tool),
			Source:     p.Tool.Source,
			Endpoint:   p.Tool.Endpoint,
		}
		if !p.StartAt.IsZero() && (st == Running || st == External) {
			v.Uptime = humanUptime(p.StartAt)
		}
		p.mu.Unlock()
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, 200, map[string]any{
		"tools":     out,
		"listen":    s.cfg.cfg.Listen.Host + ":" + strconv.Itoa(s.cfg.cfg.Listen.Port),
		"workspace": s.cfg.cfg.Workspace,
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func humanUptime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	default:
		return strconv.Itoa(int(d.Hours())) + "h" + strconv.Itoa(int(d.Minutes())%60) + "m"
	}
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(indexHTML)
}

// 监听失败时给可操作提示，而不是裸的 bind: address already in use
func listenHint(err error, port int) string {
	if err == nil {
		return ""
	}
	if strings.Contains(err.Error(), "address already in use") {
		return fmt.Sprintf("端口 %d 已被占用。\n\n"+
			"如果工具箱已经在跑，直接用浏览器打开 http://127.0.0.1:%d\n"+
			"要重启就先关掉旧的：  pkill -f toolbox  或  toolbox -stop\n\n"+
			"查看占用进程：  lsof -nP -iTCP:%d -sTCP:LISTEN", port, port, port)
	}
	return err.Error()
}

func (s *server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="toolbox-bootstrap.sh"`)
	_, _ = w.Write([]byte(s.bootstrapScript()))
}

func (s *server) bootstrapScript() string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n")
	b.WriteString("# 换机器恢复工具箱。在目标机器上执行：bash toolbox-bootstrap.sh\n")
	b.WriteString("set -euo pipefail\n\n")
	b.WriteString("WS=" + sh(s.cfg.cfg.Workspace) + "\n")
	b.WriteString("mkdir -p \"$HOME/.toolbox/logs\"\n\n")
	b.WriteString("mkdir -p \"$(dirname \"$WS\")\"\n")
	b.WriteString("[ -d \"$WS/tools/.git\" ] || git clone git@github.com:xuxinzhi007/tools.git \"$WS/tools\"\n")
	for _, t := range s.cfg.tools.Tools {
		if t.Dir == "" {
			continue
		}
		b.WriteString("[ -d \"$WS/tools/" + t.Dir + "\" ] || echo \"!! 缺 " + t.Dir + "\"\n")
	}
	b.WriteString("\n# 二进制不再进版本库，需要时现编：\n")
	b.WriteString("cd \"$WS/tools/toolbox\" && go build -o \"$HOME/bin/toolbox\" .\n")
	b.WriteString("cd \"$WS/tools/sql-monitor\" && go build -o /dev/null ./cmd/sql-monitor\n")
	b.WriteString("cd \"$WS/tools/system_tags_editor\" && go vet .\n")
	b.WriteString("\n# MCP 依赖\n")
	for _, t := range s.cfg.tools.Tools {
		if t.Group == "mcp" {
			b.WriteString("cd \"$WS/tools/" + t.Dir + "\" && uv sync\n")
		}
	}
	b.WriteString("\necho '需要手动恢复: $HOME/.toolbox/config.yaml 与 tools.yaml（含凭证，不要进 git）'\n")
	return b.String()
}

func sh(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func ensureDir(p string) {
	_ = os.MkdirAll(p, 0o700)
}
