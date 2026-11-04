package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const mcpServerInstructions = `你是 go-file 协作助手，工作区根目录为 AI 专属 workspace/。
开始任务前请先调用 get_workspace_info，并读取 collab/active-task.json。
- 方案、笔记、测试用例请写在 workspace 内（推荐 collab/plans/、notes/、tests/）
- 可自由创建目录和文件
- 仅允许访问 workspace 内的路径，不可访问服务代码（*.go、config.json 等）`

var workspaceSubdirs = []string{
	"collab/plans",
	"collab/decisions",
	"notes",
	"tests",
	"scratch",
}

func ensureWorkspace(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	for _, sub := range workspaceSubdirs {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", sub, err)
		}
	}

	readme := filepath.Join(root, "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		content := `# AI 工作区

此目录供 Grok 等 AI 通过 MCP 自由读写，与服务代码隔离。

## 目录说明

| 目录 | 用途 |
|------|------|
| collab/ | 协同任务与状态 |
| collab/plans/ | 方案文档 |
| collab/decisions/ | 已确定决策 |
| notes/ | 临时笔记 |
| tests/ | 接口测试 JSON |
| scratch/ | 草稿与实验 |

## 开始协作

1. 读取 collab/active-task.json
2. 方案写入 collab/plans/
3. 完成后更新 active-task.json 的 phase 字段
`
		if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
			return err
		}
	}

	taskFile := filepath.Join(root, "collab", "active-task.json")
	if _, err := os.Stat(taskFile); os.IsNotExist(err) {
		task := map[string]any{
			"id":        "",
			"phase":     "idle",
			"title":     "",
			"plan_file": "",
			"notes":     "AI 可在此工作区自由创建文件。开始新任务时请更新此文件。",
		}
		b, _ := json.MarshalIndent(task, "", "  ")
		if err := os.WriteFile(taskFile, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func workspaceInfoText() (string, bool) {
	type dirEntry struct {
		Path string `json:"path"`
		Desc string `json:"desc"`
	}
	info := map[string]any{
		"workspace_root": baseDir,
		"instructions": []string{
			"所有 MCP 文件操作仅限此工作区内",
			"开始任务前先读 collab/active-task.json",
			"方案写入 collab/plans/，笔记写入 notes/",
		},
		"directories": []dirEntry{
			{Path: "collab/", Desc: "协同任务与 active-task.json"},
			{Path: "collab/plans/", Desc: "方案文档"},
			{Path: "collab/decisions/", Desc: "已定决策，避免反复修改"},
			{Path: "notes/", Desc: "临时笔记"},
			{Path: "tests/", Desc: "接口测试 JSON"},
			{Path: "scratch/", Desc: "草稿与实验"},
		},
	}

	if raw, err := os.ReadFile(filepath.Join(baseDir, "collab", "active-task.json")); err == nil {
		var task map[string]any
		if json.Unmarshal(raw, &task) == nil {
			info["active_task"] = task
		}
	}

	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err.Error(), true
	}
	return string(b), false
}
