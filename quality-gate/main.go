package main

import (
	"log"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func main() {
	appConfig = loadConfig()

	baseDir = appConfig.Server.Root
	authToken = appConfig.Server.AuthToken
	port := appConfig.serverPort()

	absRoot, err := filepath.Abs(baseDir)
	if err != nil {
		log.Fatalf("resolve root: %v", err)
	}
	baseDir = absRoot

	if err := ensureWorkspace(baseDir); err != nil {
		log.Fatalf("init workspace: %v", err)
	}

	initDashboard()
	log.Print(appConfig.configSummary())

	maybeStartNgrok(port)
	startNgrokWatcher()

	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusPermanentRedirect, "/static/dashboard.html")
	})
	r.StaticFS("/static", http.Dir("./static"))

	registerDashboardRoutes(r)
	registerWorkspaceRoutes(r)
	registerMCPRoutes(r)

	log.Printf("MCP server on :%s, workspace=%s, auth=%t", port, baseDir, authToken != "")
	log.Printf("dashboard: http://localhost:%s/static/dashboard.html", port)
	log.Printf("endpoints: POST/GET/DELETE /mcp, GET/POST /sse, POST /messages")
	log.Fatal(r.Run(":" + port))
}
