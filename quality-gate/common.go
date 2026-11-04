package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const protocolVersion = "2025-03-26"

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type session struct {
	id string
}

var (
	baseDir   string
	authToken string
	sessions  sync.Map
)

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func authMiddleware(c *gin.Context) {
	if authToken == "" {
		c.Next()
		return
	}
	header := c.GetHeader("Authorization")
	if header != "Bearer "+authToken {
		c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
		return
	}
	c.Next()
}

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSONRPCError(c *gin.Context, id json.RawMessage, code int, message string) {
	c.JSON(200, gin.H{
		"jsonrpc": "2.0",
		"id":      id,
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func safePath(relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" {
		return filepath.Clean(baseDir), nil
	}
	clean := filepath.Clean(filepath.Join(baseDir, relPath))
	root := filepath.Clean(baseDir)
	if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("path not allowed: must stay inside workspace")
	}
	return clean, nil
}

func toolListFiles(relPath string) (string, bool) {
	dir, err := safePath(relPath)
	if err != nil {
		return err.Error(), true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err.Error(), true
	}

	type item struct {
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size,omitempty"`
	}
	items := make([]item, 0, len(entries))
	for _, e := range entries {
		it := item{Name: e.Name(), IsDir: e.IsDir()}
		if !e.IsDir() {
			if info, err := e.Info(); err == nil {
				it.Size = info.Size()
			}
		}
		items = append(items, it)
	}
	b, _ := json.MarshalIndent(items, "", "  ")
	return string(b), false
}

func toolSearchFiles(pattern, query string, maxResults int) (string, bool) {
	if maxResults <= 0 {
		maxResults = 50
	}
	if pattern == "" {
		pattern = "*"
	}

	var matches []string
	_ = filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		name := d.Name()
		ok, _ := filepath.Match(pattern, name)
		if !ok {
			return nil
		}
		if query != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
			return nil
		}
		matches = append(matches, rel)
		if len(matches) >= maxResults {
			return filepath.SkipAll
		}
		return nil
	})

	b, _ := json.MarshalIndent(matches, "", "  ")
	return string(b), false
}

func toolReadFile(relPath string, offset, limit int) (string, bool) {
	path, err := safePath(relPath)
	if err != nil {
		return err.Error(), true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err.Error(), true
	}
	content := string(data)
	if offset <= 0 && limit <= 0 {
		return content, false
	}

	lines := strings.Split(content, "\n")
	start := 0
	if offset > 0 {
		start = offset - 1
	}
	if start >= len(lines) {
		return "", false
	}
	end := len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&b, "%4d| %s\n", i+1, lines[i])
	}
	return b.String(), false
}

func toolWriteFile(relPath, content string) (string, bool) {
	path, err := safePath(relPath)
	if err != nil {
		return err.Error(), true
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err.Error(), true
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("file written: %s (%d bytes)", filepath.ToSlash(relPath), len(content)), false
}

func toolAppendFile(relPath, content string) (string, bool) {
	path, err := safePath(relPath)
	if err != nil {
		return err.Error(), true
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err.Error(), true
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err.Error(), true
	}
	defer f.Close()
	n, err := f.WriteString(content)
	if err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("appended %d bytes to %s", n, filepath.ToSlash(relPath)), false
}

func toolUpdateActiveTask(id, phase, title, planFile, notes string) (string, bool) {
	taskPath := filepath.Join(baseDir, "collab", "active-task.json")
	task := map[string]any{}
	if raw, err := os.ReadFile(taskPath); err == nil {
		_ = json.Unmarshal(raw, &task)
	}
	if id != "" {
		task["id"] = id
	}
	if phase != "" {
		task["phase"] = phase
	}
	if title != "" {
		task["title"] = title
	}
	if planFile != "" {
		task["plan_file"] = planFile
	}
	if notes != "" {
		task["notes"] = notes
	}
	task["updated_at"] = time.Now().Format(time.RFC3339)

	b, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err.Error(), true
	}
	if err := os.WriteFile(taskPath, b, 0o644); err != nil {
		return err.Error(), true
	}
	return string(b), false
}
