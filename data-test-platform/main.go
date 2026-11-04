package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"data-test-platform/internal/config"
	"data-test-platform/internal/logger"
	"data-test-platform/internal/openapi"
	"data-test-platform/internal/pkg/directnet"
	"data-test-platform/internal/proxy"
	"data-test-platform/internal/server"
)

//go:embed web/dist/*
var staticFS embed.FS

func main() {
	port := flag.Int("port", 9099, "Web server listening port")
	openAPIPath := flag.String("openapi", "openapi.yaml", "Path to OpenAPI 3.0 specification file")
	logFilePath := flag.String("log", "app.log", "Path to application log file to tail")
	targetURL := flag.String("target", "", "Default target service address (empty = use environment from --project)")
	slowThresholdStr := flag.String("slow-threshold", "200ms", "Threshold for slow SQL marking (e.g. 200ms, 500ms)")
	k8sConfigPath := flag.String("k8s-config", "", "Optional JSON file enabling the K8s log source (omit to disable)")
	projectPath := flag.String("project", "", "Optional JSON project profile (environments, headers, remote log, project name)")
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
	// K8s 日志源是可选加载：不传 --k8s-config 时永远不启动 kubectl。
	if *k8sConfigPath != "" {
		if err := config.LoadK8sConfig(*k8sConfigPath, &cfg.K8s); err != nil {
			log.Fatalf("\u274c 加载 K8s 配置失败 %s: %v", *k8sConfigPath, err)
		}
	}

	// 项目 profile：域名、环境、鉴权头都在这里，代码本身不绑定任何项目。
	profile, err := config.LoadProfile(*projectPath)
	if err != nil {
		log.Fatalf("\u274c 加载项目配置失败 %s: %v", *projectPath, err)
	}
	cfg.ApplyProfile(profile)

	// 内部域名直连规则由 profile 提供，代码里不含任何企业域名。
	directnet.SetExtraSuffixes(cfg.DirectHosts)

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
	k8sTailer := logger.NewK8sTailer(hub)

	srv := server.NewServer(server.Deps{
		Config:       cfg,
		Parser:       parser,
		Proxy:        proxyClient,
		Hub:          hub,
		Tailer:       tailer,
		Runner:       runner,
		RemotePoller: remotePoller,
		K8sTailer:    k8sTailer,
		StaticFS:     staticFS,
	})

	addr := fmt.Sprintf(":%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("❌ Failed to bind address %s: %v", addr, err)
	}

	// 启动前探测可用日志源：把 "auto" 真正落实成具体来源，
	// 否则页面打开后日志区是空的，用户得自己去找下拉框。
	logFileExists := false
	if st, err := os.Stat(cfg.LogFilePath); err == nil && !st.IsDir() {
		logFileExists = true
	}
	chosen, sourceStates := cfg.ProbeSources(logFileExists)
	if chosen != "none" {
		cfg.SetLogSource(chosen)
	}

	// Start log monitors
	switch chosen {
	case "local":
		tailer.Start()
	case "dev", "test", "prod":
		remotePoller.Start()
	}

	fmt.Println()
	fmt.Println("🚀 =================================================================")
	fmt.Printf("   %s · 接口测试与日志 SQL 实时监听\n", cfg.ProjectName)
	fmt.Println("=================================================================")
	fmt.Printf("🌐 Web 访问界面:       http://localhost:%d\n", *port)
	fmt.Printf("🎯 默认测试目标服务:   %s\n", cfg.GetBaseURL())
	fmt.Printf("📄 OpenAPI 规范文件:   %s\n", cfg.OpenAPIPath)
	fmt.Printf("🐢 慢 SQL 判定阈值:    %s\n", slowThreshold)
	fmt.Println("-----------------------------------------------------------------")
	if chosen == "none" {
		fmt.Println("📜 日志源:             ❌ 没有可用的源")
	} else {
		fmt.Printf("📜 日志源:             %s\n", chosen)
	}
	for _, st := range sourceStates {
		mark := "  "
		if st.ID == chosen {
			mark = "→ "
		} else if st.Available {
			mark = "✓ "
		}
		fmt.Printf("   %s%-6s %s\n", mark, st.ID, st.Reason)
	}
	if chosen == "none" {
		fmt.Println("   提示: 用 --log 指定日志文件，或在页面里配置云端 token / --k8s-config")
	}
	fmt.Println("=================================================================")
	fmt.Println("💡 Ctrl+K 搜接口 · Ctrl+J 搜日志 · Ctrl+Enter 发送 · Esc 关弹窗")
	fmt.Println()
	fmt.Println("   按 Ctrl+C 即可退出")
	fmt.Println()

	httpServer := &http.Server{
		Handler: srv.Routes(),
	}

	// kubectl 是子进程，必须显式收，否则父进程退出后它会变成孤儿。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		fmt.Println("\n正在退出…")
		k8sTailer.Stop()
		tailer.Stop()
		remotePoller.Stop()
		_ = httpServer.Close()
	}()

	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server stopped: %v", err)
	}
}
