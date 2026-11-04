package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type workspaceEntry struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

func registerWorkspaceRoutes(r *gin.Engine) {
	r.GET("/api/workspace/list", func(c *gin.Context) {
		sub := c.Query("path")
		dir, err := safePath(sub)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		items := make([]workspaceEntry, 0, len(entries))
		for _, e := range entries {
			it := workspaceEntry{
				Name:  e.Name(),
				IsDir: e.IsDir(),
				Path:  filepath.ToSlash(filepath.Join(sub, e.Name())),
			}
			if !e.IsDir() {
				if info, err := e.Info(); err == nil {
					it.Size = info.Size()
				}
			}
			items = append(items, it)
		}
		c.JSON(http.StatusOK, gin.H{
			"path":    filepath.ToSlash(sub),
			"entries": items,
		})
	})

	r.GET("/api/workspace/tree", func(c *gin.Context) {
		max := 200
		var files []workspaceEntry
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
			info, err := d.Info()
			if err != nil {
				return nil
			}
			files = append(files, workspaceEntry{
				Path:  filepath.ToSlash(rel),
				Name:  d.Name(),
				IsDir: false,
				Size:  info.Size(),
			})
			if len(files) >= max {
				return filepath.SkipAll
			}
			return nil
		})
		c.JSON(http.StatusOK, gin.H{"files": files, "count": len(files)})
	})

	r.GET("/api/workspace/file", func(c *gin.Context) {
		rel := strings.TrimSpace(c.Query("path"))
		if rel == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
			return
		}
		path, err := safePath(rel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		info, err := os.Stat(path)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if info.IsDir() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "path is a directory"})
			return
		}
		if info.Size() > 512*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file too large for preview (>512KB)"})
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"path":    filepath.ToSlash(rel),
			"size":    info.Size(),
			"content": string(data),
		})
	})
}
