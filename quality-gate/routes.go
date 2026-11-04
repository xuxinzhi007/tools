package main

import (
	"github.com/gin-gonic/gin"
)

func registerMCPRoutes(r *gin.Engine) {
	mcp := r.Group("")
	mcp.Use(authMiddleware)

	// Streamable HTTP (2025-03-26)
	mcp.POST("/mcp", handleMCPPost)
	mcp.GET("/mcp", handleMCPGet)
	mcp.DELETE("/mcp", handleMCPDelete)

	// Legacy HTTP+SSE (2024-11-05) — Grok may fall back to this
	mcp.GET("/sse", handleLegacySSEGet)
	mcp.POST("/sse", handleSSEPost)
	mcp.POST("/messages", handleLegacySSEMessage)
}
