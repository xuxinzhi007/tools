package main

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
)

func toolDefinitions() []toolDef {
	return []toolDef{
		{
			Name:        "get_workspace_info",
			Description: "获取 AI 工作区说明、目录约定与当前 active-task。开始任何任务前应先调用此工具。",
			InputSchema: gin.H{
				"type":       "object",
				"properties": gin.H{},
			},
		},
		{
			Name:        "list_files",
			Description: "列出工作区内某目录下的文件和子目录。path 为空表示根目录，例如 collab/plans",
			InputSchema: gin.H{
				"type": "object",
				"properties": gin.H{
					"path": gin.H{
						"type":        "string",
						"description": "相对工作区的目录路径，留空表示根目录",
					},
				},
			},
		},
		{
			Name:        "search_files",
			Description: "在工作区内搜索文件。可按文件名 glob（如 *.json）或名称关键字过滤。",
			InputSchema: gin.H{
				"type": "object",
				"properties": gin.H{
					"pattern": gin.H{
						"type":        "string",
						"description": "文件名 glob，例如 *.md、*.json，默认 *",
					},
					"query": gin.H{
						"type":        "string",
						"description": "文件名包含的关键字（可选）",
					},
					"max_results": gin.H{
						"type":        "integer",
						"description": "最多返回条数，默认 50",
					},
				},
			},
		},
		{
			Name:        "read_file",
			Description: "读取工作区内的文件内容。大文件可用 offset/limit 按行分段读取。",
			InputSchema: gin.H{
				"type": "object",
				"properties": gin.H{
					"path": gin.H{
						"type":        "string",
						"description": "相对工作区的文件路径，例如 collab/active-task.json",
					},
					"offset": gin.H{
						"type":        "integer",
						"description": "起始行号（从 1 开始，可选）",
					},
					"limit": gin.H{
						"type":        "integer",
						"description": "最多读取行数（可选）",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "write_file",
			Description: "在工作区内创建或覆盖文件。可自动创建父目录。用于方案、笔记、测试配置等。",
			InputSchema: gin.H{
				"type": "object",
				"properties": gin.H{
					"path": gin.H{
						"type":        "string",
						"description": "相对工作区的文件路径",
					},
					"content": gin.H{
						"type":        "string",
						"description": "文件完整内容",
					},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "append_file",
			Description: "向工作区内已有文件末尾追加内容；文件不存在则创建。",
			InputSchema: gin.H{
				"type": "object",
				"properties": gin.H{
					"path": gin.H{
						"type":        "string",
						"description": "相对工作区的文件路径",
					},
					"content": gin.H{
						"type":        "string",
						"description": "要追加的内容",
					},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "update_active_task",
			Description: "更新 collab/active-task.json 中的协同任务状态（phase/title/plan_file 等）。",
			InputSchema: gin.H{
				"type": "object",
				"properties": gin.H{
					"id": gin.H{"type": "string", "description": "任务 ID"},
					"phase": gin.H{
						"type":        "string",
						"description": "idle | planning | implementing | review | done",
					},
					"title":     gin.H{"type": "string", "description": "任务标题"},
					"plan_file": gin.H{"type": "string", "description": "方案文件相对路径"},
					"notes":     gin.H{"type": "string", "description": "备注"},
				},
			},
		},
	}
}

type toolCallDetail struct {
	tool    string
	summary string
	isError bool
}

func executeTool(name string, args json.RawMessage) (text string, isError bool, detail toolCallDetail) {
	detail.tool = name

	switch name {
	case "get_workspace_info":
		detail.summary = "workspace info"
		text, isError := workspaceInfoText()
		return text, isError, detail

	case "list_files":
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &a)
		detail.summary = fmt.Sprintf("path=%q", a.Path)
		text, isError = toolListFiles(a.Path)
		return text, isError, detail

	case "search_files":
		var a struct {
			Pattern    string `json:"pattern"`
			Query      string `json:"query"`
			MaxResults int    `json:"max_results"`
		}
		_ = json.Unmarshal(args, &a)
		detail.summary = fmt.Sprintf("pattern=%q query=%q", a.Pattern, a.Query)
		text, isError = toolSearchFiles(a.Pattern, a.Query, a.MaxResults)
		return text, isError, detail

	case "read_file":
		var a struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		_ = json.Unmarshal(args, &a)
		detail.summary = fmt.Sprintf("path=%q", a.Path)
		text, isError = toolReadFile(a.Path, a.Offset, a.Limit)
		return text, isError, detail

	case "write_file":
		var a struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		_ = json.Unmarshal(args, &a)
		detail.summary = fmt.Sprintf("path=%q bytes=%d", a.Path, len(a.Content))
		text, isError = toolWriteFile(a.Path, a.Content)
		return text, isError, detail

	case "append_file":
		var a struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		_ = json.Unmarshal(args, &a)
		detail.summary = fmt.Sprintf("path=%q append=%d", a.Path, len(a.Content))
		text, isError = toolAppendFile(a.Path, a.Content)
		return text, isError, detail

	case "update_active_task":
		var a struct {
			ID       string `json:"id"`
			Phase    string `json:"phase"`
			Title    string `json:"title"`
			PlanFile string `json:"plan_file"`
			Notes    string `json:"notes"`
		}
		_ = json.Unmarshal(args, &a)
		detail.summary = fmt.Sprintf("phase=%q title=%q", a.Phase, a.Title)
		text, isError = toolUpdateActiveTask(a.ID, a.Phase, a.Title, a.PlanFile, a.Notes)
		return text, isError, detail

	default:
		detail.summary = "unknown tool"
		return fmt.Sprintf("unknown tool: %s", name), true, detail
	}
}

func toolsCallResult(req jsonRPCRequest) (gin.H, bool, toolCallDetail, string) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, false, toolCallDetail{tool: "unknown"}, "invalid params"
	}

	text, isError, detail := executeTool(params.Name, params.Arguments)
	errMsg := ""
	if isError {
		errMsg = text
	}

	if detail.tool != params.Name && params.Name != "" {
		detail.tool = params.Name
	}

	result := gin.H{
		"content": []gin.H{
			{"type": "text", "text": text},
		},
	}
	if isError {
		result["isError"] = true
	}
	return gin.H{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result":  result,
	}, true, detail, errMsg
}
