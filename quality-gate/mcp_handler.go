package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

type mcpResponse struct {
	status int
	body   any
}

func dispatchMCP(ctx mcpCallContext, req jsonRPCRequest) mcpResponse {
	switch req.Method {
	case "initialize":
		sessionID := newSessionID()
		sessions.Store(sessionID, &session{id: sessionID})
		recordSessionCreated(ctx)
		return mcpResponse{
			status: http.StatusOK,
			body: gin.H{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": gin.H{
					"protocolVersion": protocolVersion,
					"capabilities": gin.H{
						"tools": gin.H{"listChanged": false},
					},
					"serverInfo": gin.H{
						"name":    "go-file-mcp",
						"version": serverVersion,
					},
					"instructions": mcpServerInstructions,
				},
				"_sessionID": sessionID,
			},
		}
	case "notifications/initialized", "initialized":
		recordMCPRequest(ctx, req.Method, true, "client ready")
		return mcpResponse{status: http.StatusAccepted, body: nil}
	case "tools/list":
		recordMCPRequest(ctx, req.Method, true, fmt.Sprintf("%d tools", len(toolDefinitions())))
		return mcpResponse{
			status: http.StatusOK,
			body: gin.H{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  toolsListResult(),
			},
		}
	case "tools/call":
		body, ok, detail, errMsg := toolsCallResult(req)
		recordToolCall(ctx, detail.tool, ok && !detail.isError, detail.summary, errMsg)
		if !ok {
			return mcpResponse{
				status: http.StatusOK,
				body: gin.H{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"error": gin.H{
						"code":    -32602,
						"message": "invalid params",
					},
				},
			}
		}
		return mcpResponse{status: http.StatusOK, body: body}
	case "ping":
		recordMCPRequest(ctx, req.Method, true, "keepalive")
		return mcpResponse{
			status: http.StatusOK,
			body: gin.H{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  gin.H{},
			},
		}
	default:
		recordMCPRequest(ctx, req.Method, false, "unknown method")
		return mcpResponse{
			status: http.StatusOK,
			body: gin.H{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error": gin.H{
					"code":    -32601,
					"message": "method not found: " + req.Method,
				},
			},
		}
	}
}

func toolsListResult() gin.H {
	tools := make([]gin.H, 0, len(toolDefinitions()))
	for _, def := range toolDefinitions() {
		tools = append(tools, gin.H{
			"name":        def.Name,
			"description": def.Description,
			"inputSchema": def.InputSchema,
		})
	}
	return gin.H{"tools": tools}
}

func writeMCPResponse(c *gin.Context, resp mcpResponse) {
	if resp.status == http.StatusAccepted {
		c.Status(http.StatusAccepted)
		return
	}

	body, ok := resp.body.(gin.H)
	if !ok {
		c.JSON(resp.status, resp.body)
		return
	}

	if sessionID, ok := body["_sessionID"].(string); ok {
		c.Header("Mcp-Session-Id", sessionID)
		delete(body, "_sessionID")
	}

	c.JSON(resp.status, body)
}

func parseMCPRequest(c *gin.Context) (jsonRPCRequest, bool) {
	var req jsonRPCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeJSONRPCError(c, nil, -32700, "parse error")
		return jsonRPCRequest{}, false
	}
	return req, true
}

func handleMCPPost(c *gin.Context) {
	req, ok := parseMCPRequest(c)
	if !ok {
		return
	}
	writeMCPResponse(c, dispatchMCP(newMCPContext(c, "streamable-http"), req))
}

func handleMCPGet(c *gin.Context) {
	incMCPStreams(1)
	defer incMCPStreams(-1)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	c.Writer.Flush()
	<-c.Request.Context().Done()
}

func handleMCPDelete(c *gin.Context) {
	ctx := newMCPContext(c, "streamable-http")
	sessionID := c.GetHeader("Mcp-Session-Id")
	if sessionID != "" {
		sessions.Delete(sessionID)
		recordMCPRequest(ctx, "session/delete", true, "session="+sessionID[:8]+"...")
	} else {
		recordMCPRequest(ctx, "session/delete", true, "no session id")
	}
	c.Status(http.StatusNoContent)
}

type sseLegacySession struct {
	id     string
	outbox chan []byte
}

var sseSessions sync.Map

func handleLegacySSEGet(c *gin.Context) {
	incSSEConnections(1)
	defer incSSEConnections(-1)

	sessionID := newSessionID()
	sess := &sseLegacySession{
		id:     sessionID,
		outbox: make(chan []byte, 8),
	}
	sseSessions.Store(sessionID, sess)
	defer sseSessions.Delete(sessionID)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	endpoint := fmt.Sprintf("/messages?sessionId=%s", sessionID)
	fmt.Fprintf(c.Writer, "event: endpoint\ndata: %s\n\n", endpoint)
	c.Writer.Flush()

	for {
		select {
		case msg := <-sess.outbox:
			fmt.Fprintf(c.Writer, "event: message\ndata: %s\n\n", msg)
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}

func handleSSEPost(c *gin.Context) {
	req, ok := parseMCPRequest(c)
	if !ok {
		return
	}
	writeMCPResponse(c, dispatchMCP(newMCPContext(c, "streamable-http"), req))
}

func handleLegacySSEMessage(c *gin.Context) {
	sessionID := c.Query("sessionId")
	val, ok := sseSessions.Load(sessionID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	sess := val.(*sseLegacySession)

	req, ok := parseMCPRequest(c)
	if !ok {
		return
	}

	resp := dispatchMCP(newMCPContext(c, "legacy-sse"), req)
	if resp.status == http.StatusAccepted {
		c.Status(http.StatusAccepted)
		return
	}

	body, ok := resp.body.(gin.H)
	if !ok {
		c.Status(http.StatusAccepted)
		return
	}
	if sid, ok := body["_sessionID"].(string); ok {
		sessions.Store(sid, &session{id: sid})
		delete(body, "_sessionID")
	}

	data, err := json.Marshal(body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "marshal failed"})
		return
	}

	select {
	case sess.outbox <- data:
		c.Status(http.StatusAccepted)
	default:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sse buffer full"})
	}
}
