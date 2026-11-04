package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed index.html
var indexHTML []byte

//go:embed pretrain-label-resync.html
var resyncHTML []byte

//go:embed label-tag-dirty-data-repair.html
var repairHTML []byte

//go:embed tag-qc-list-to-init.html
var qcInitHTML []byte

var tagsPath string

func writeJSON(w http.ResponseWriter, code int, obj any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	fmt.Fprint(w, objJSON(obj))
}

func objJSON(obj any) string {
	switch t := obj.(type) {
	case map[string]any:
		// 保持固定键顺序，便于阅读
		var buf bytes.Buffer
		buf.WriteByte('{')
		first := true
		for _, k := range []string{"ok", "path", "error", "data", "text"} {
			v, ok := t[k]
			if !ok {
				continue
			}
			if !first {
				buf.WriteString(", ")
			}
			first = false
			buf.WriteString(marshalKey(k) + ": ")
			buf.WriteString(inline(v))
		}
		buf.WriteByte('}')
		return buf.String()
	default:
		return inline(obj)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		writeJSON(w, 404, map[string]any{"error": "not found"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(indexHTML)
}

func handleLoad(w http.ResponseWriter, r *http.Request) {
	raw, err := os.ReadFile(tagsPath)
	if err != nil {
		writeJSON(w, 404, map[string]any{"error": "file not found: " + tagsPath})
		return
	}
	data, err := ParseOrdered(bytes.NewReader(raw))
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"path": tagsPath, "data": data, "text": string(raw)})
}

func handleSaveOrFormat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 404, map[string]any{"error": "not found"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	parsed, err := ParseOrdered(bytes.NewReader(body))
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	payload, ok := parsed.(*OrderedMap)
	data, hasData := payload.M["data"]
	if !ok || !hasData {
		writeJSON(w, 400, map[string]any{"error": "missing data"})
		return
	}
	// 先清理编辑态空值，再做仓库紧凑风格序列化 + round-trip 语义校验
	CleanTagData(data)
	text := DumpCompact(data)
	if !RoundTripOK(text, data) {
		writeJSON(w, 400, map[string]any{"error": "compact serializer round-trip mismatch"})
		return
	}
	if r.URL.Path == "/api/format" {
		writeJSON(w, 200, map[string]any{"text": text})
		return
	}
	if err := os.MkdirAll(filepath.Dir(tagsPath), 0o755); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	if err := writeFileAtomic(tagsPath, []byte(text), 0o644); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "path": tagsPath})
}

// writeFileAtomic 写文件要么完整成功，要么原文件一字不动。
//
// system_tags.json 是跨仓库共享的标签配置，被 data-service 直接读。
// 旧的 os.WriteFile 会先截断再写：进程被杀、磁盘满、权限不足都会
// 留下一个半个文件——那不是“保存失败”，是配置被弄坏了。
//
// 步骤：同目录临时文件 → 写 → fsync → 改权限 → rename。
// rename 在同一文件系统内是原子的，所以读方要么看到旧内容，要么看到新内容。
// 同目录是必须的：跨文件系统 rename 会退化成 copy，不再原子。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// 提前清理：任何一步失败都不留垃圾文件。
	// 成功路径下 rename 已把 tmpName 移走，Remove 返回 ENOENT 无副作用。
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	// CreateTemp 建出来是 0600，不显式改的话落盘权限会跟着变。
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func lanIP() string {
	c, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

// parseDeviceModels 从 data-service config.app.yaml 扫描 supported_device_models 的 code 列表，
// 供编辑器校验 device_models 拼写；文件缺失或解析不到时返回 nil（降级为仅文件内型号提示）。
func parseDeviceModels(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var models []string
	inBlock := false
	blockIndent := 0
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if strings.HasPrefix(trimmed, "supported_device_models:") {
				inBlock = true
				blockIndent = len(line) - len(strings.TrimLeft(line, " "))
			}
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if indent := len(line) - len(strings.TrimLeft(line, " ")); indent <= blockIndent {
			break
		}
		if strings.HasPrefix(trimmed, "- code:") {
			models = append(models, strings.TrimSpace(strings.TrimPrefix(trimmed, "- code:")))
		}
	}
	return models
}

func deviceConfigPath() string {
	// tagsPath = <repo>/configs/tag/system_tags.json → config.app.yaml 在同级 configs 下
	return filepath.Join(filepath.Dir(filepath.Dir(tagsPath)), "config.app.yaml")
}

func parseNoBody(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "no_body:") {
			continue
		}
		rest := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "no_body:")), "[]")
		var out []string
		for _, p := range strings.Split(rest, ",") {
			p = strings.Trim(p, " \"'")
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return nil
}

func handleDeviceModels(w http.ResponseWriter, r *http.Request) {
	// inline 序列化仅识别 []any，[]string 会落入 fmt 默认格式
	raw := parseDeviceModels(deviceConfigPath())
	models := make([]any, len(raw))
	for i, m := range raw {
		models[i] = m
	}
	writeJSON(w, 200, map[string]any{"data": models})
}

func handleDeviceClasses(w http.ResponseWriter, r *http.Request) {
	cfg := deviceConfigPath()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"models":  parseDeviceModels(cfg),
		"no_body": parseNoBody(cfg),
	})
}

func handleHTML(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func main() {
	host := flag.String("host", "0.0.0.0", "listen host")
	port := flag.Int("port", 8765, "listen port")
	file := flag.String("file", filepath.Join("..", "..", "data-service", "configs", "tag", "system_tags.json"), "path to system_tags.json")
	flag.Parse()

	abs, err := filepath.Abs(*file)
	if err != nil {
		abs = *file
	}
	tagsPath = abs

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/tools/pretrain-resync", func(w http.ResponseWriter, r *http.Request) {
		handleHTML(w, resyncHTML)
	})
	http.HandleFunc("/tools/label-repair", func(w http.ResponseWriter, r *http.Request) {
		handleHTML(w, repairHTML)
	})
	http.HandleFunc("/tools/qc-init", func(w http.ResponseWriter, r *http.Request) {
		handleHTML(w, qcInitHTML)
	})
	http.HandleFunc("/api/load", handleLoad)
	http.HandleFunc("/api/save", handleSaveOrFormat)
	http.HandleFunc("/api/format", handleSaveOrFormat)
	http.HandleFunc("/api/device-models", handleDeviceModels)
	http.HandleFunc("/api/device-classes", handleDeviceClasses)

	addr := fmt.Sprintf("%s:%d", *host, *port)
	fmt.Printf("open http://%s\n", addr)
	if *host == "0.0.0.0" {
		if ip := lanIP(); ip != "" {
			fmt.Printf("LAN  http://%s:%d\n", ip, *port)
		}
	}
	fmt.Println("tags file:", tagsPath)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Println("server error:", err)
		os.Exit(1)
	}
}
