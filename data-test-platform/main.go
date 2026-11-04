package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"data-test-platform/internal/config"
	"data-test-platform/internal/logger"
	"data-test-platform/internal/openapi"
	"data-test-platform/internal/proxy"
	"data-test-platform/internal/server"
)

//go:embed web/dist/*
var staticFS embed.FS

func main() {
	port := flag.Int("port", 9099, "Web server listening port")
	openAPIPath := flag.String("openapi", "../data-service/openapi.yaml", "Path to OpenAPI 3.0 specification file")
	logFilePath := flag.String("log", "../data-service/logs/app.log", "Path to data-service application log file to tail")
	targetURL := flag.String("target", "", "Default target data-service address (empty = auto-detect from configs/config.app.local.yaml)")
	slowThresholdStr := flag.String("slow-threshold", "200ms", "Threshold for slow SQL marking (e.g. 200ms, 500ms)")
	flag.Parse()

	slowThreshold, err := time.ParseDuration(*slowThresholdStr)
	if err != nil {
		slowThreshold = 200 * time.Millisecond
	}

	cfg := config.NewDefaultConfig()
	cfg.Port = *port
	cfg.OpenAPIPath = *openAPIPath
	cfg.LogFilePath = *logFilePath
	cfg.SlowSQLThreshold = slowThreshold
	if *targetURL != "" {
		cfg.CustomBaseURL = *targetURL
		cfg.CurrentEnvID = "local"
	}

	parser := openapi.NewParser(cfg.OpenAPIPath)
	proxyClient := proxy.NewProxy()

	logParser := logger.NewLogParser(slowThreshold)
	hub := logger.NewHub(logParser)
	// 本地日志入口门控:远端云日志来源 (dev/test/prod) 激活时丢弃本地原始日志,
	// 避免本地日志(含慢 SQL)混入云端日志流与统计
	hub.SetLocalGate(func() bool {
		_, _, isRemote := cfg.GetActiveRemoteLogTarget()
		return !isRemote
	})
	tailer := logger.NewFileTailer(hub, cfg.LogFilePath)
	runner := logger.NewProcessRunner(hub)
	remotePoller := logger.NewRemoteLogPoller(cfg, hub)

	srv := server.NewServer(cfg, parser, proxyClient, hub, tailer, runner, remotePoller, staticFS)

	addr := fmt.Sprintf(":%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("❌ Failed to bind address %s: %v", addr, err)
	}

	// Start log monitors
	tailer.Start()
	remotePoller.Start()

	fmt.Println()
	fmt.Println("🚀 =================================================================")
	fmt.Println("   Data Service API 接口测试 & 日志 SQL 实时监听平台")
	fmt.Println("=================================================================")
	fmt.Printf("🌐 Web 访问界面:       http://localhost:%d\n", *port)
	fmt.Printf("🎯 默认测试目标服务:   %s (已自动读取 local 配置)\n", cfg.GetBaseURL())
	fmt.Printf("📄 OpenAPI 规范文件:   %s\n", cfg.OpenAPIPath)
	fmt.Printf("📜 日志监控文件:       %s\n", cfg.LogFilePath)
	fmt.Printf("🐢 慢 SQL 判定阈值:    %s\n", slowThreshold)
	fmt.Println("=================================================================")
	fmt.Println("💡 提示: 无需 Swagger，打开浏览器即可直接测试接口与实时捕获慢SQL！")
	fmt.Println("   按 Ctrl+C 即可退出")
	fmt.Println()

	httpServer := &http.Server{
		Handler: srv.Routes(),
	}
	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server stopped: %v", err)
	}
}
