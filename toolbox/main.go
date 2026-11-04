package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

//go:embed web/index.html
var indexHTML []byte

type server struct {
	cfg *config
	mgr *Manager
}

func main() {
	port := flag.Int("port", 0, "覆盖 config.yaml 的 listen.port")
	host := flag.String("host", "", "覆盖 listen.host；0.0.0.0 = 局域网可访问")
	noAuto := flag.Bool("no-autostart", false, "不自动启动 autostart 的工具")
	stopAll := flag.Bool("stop", false, "停掉当前在跑的工具后退出")
	flag.Parse()

	cfg, err := load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if *port != 0 {
		cfg.cfg.Listen.Port = *port
	}
	if *host != "" {
		cfg.cfg.Listen.Host = *host
		cfg.tools.ListenHost = *host
	}

	ensureDir(cfg.rootDir + "/logs")
	mgr := newManager(cfg)

	if *stopAll {
		mgr.stopAll()
		fmt.Println("已停止全部工具")
		return
	}

	if adopted, external := mgr.markListening(); adopted > 0 || external > 0 {
		if adopted > 0 {
			fmt.Printf("✓ 接管了 %d 个上次未退干净的子进程\n", adopted)
		}
		if external > 0 {
			fmt.Printf("⚠️  %d 个端口被非工具箱进程占用，卡片上可点「清理」\n", external)
		}
	}

	if !*noAuto {
		mgr.autostart()
	}
	mgr.writePID()

	srv := &server{cfg: cfg, mgr: mgr}
	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/api/", srv.handleAPI)
	mux.HandleFunc("/api/bootstrap.sh", srv.handleBootstrap)

	addr := cfg.cfg.Listen.Host + ":" + strconv.Itoa(cfg.cfg.Listen.Port)
	hs := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		fmt.Printf("工具箱  http://127.0.0.1:%d\n", cfg.cfg.Listen.Port)
		if cfg.cfg.Listen.Host == "0.0.0.0" {
			fmt.Println("⚠️  已对局域网开放")
		}
		if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "\n%s\n", listenHint(err, cfg.cfg.Listen.Port))
			stop()
		}
	}()

	<-ctx.Done()
	fmt.Println("\n正在停止子工具…")
	shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = hs.Shutdown(shutCtx)
	mgr.stopAll()
	fmt.Println("已退出")
}
