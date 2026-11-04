// Package app wires the monitor, notifier, and web UI together into the
// sql-monitor runtime. It owns CLI parsing and the lifecycle of the HTTP
// server, tailing sources, and notification goroutines. The binary entry point
// lives in cmd/sql-monitor and only sets up signal handling before delegating
// to Run.
package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"local/sql-monitor/internal/analyze"
	"local/sql-monitor/internal/config"
	"local/sql-monitor/internal/feishu"
	"local/sql-monitor/internal/monitor"
	k8s "local/sql-monitor/internal/source/k8s"
	"local/sql-monitor/internal/web"
)

// Options is the resolved runtime configuration produced by ParseOptions.
type Options struct {
	logPath      string
	port         int
	maxEvents    int
	initialBytes int64
	rawBytes     int64
	stdin        bool
	feishu       *config.FeishuConfig
	analysis     *config.AnalysisConfig
	devops       *config.DevopsConfig
	configPath   string
	k8sConfig    *config.K8sConfig
	k8s          *k8s.Target
}

// Notifier delivers aggregated SQL summaries to a notification channel.
// feishu.Notifier is the built-in implementation; alternatives can be plugged
// in by replacing the construction inside Run.
type Notifier interface {
	Run(ctx context.Context, summaries <-chan monitor.SQLSummary, output io.Writer)
}

// sourceLabel names the live input source for the feishu card title.
func sourceLabel(opts Options) string {
	switch {
	case opts.k8s != nil:
		if opts.k8s.Environment != "" {
			return "k8s " + opts.k8s.Environment
		}
		return "k8s 全部 pod"
	case opts.stdin:
		return "stdin"
	default:
		return "local"
	}
}

// k8sFlags holds raw --k8s-* values; explicitness is tracked separately so a
// zero/empty flag value can be distinguished from "not supplied".
type k8sFlags struct {
	env        string
	lane       string
	all        bool
	kubeconfig string
	context    string
	namespace  string
	app        string
	container  string
	tail       int
	kubectl    string
}

func ParseOptions(args []string, output io.Writer) (Options, error) {
	var opts Options
	var configPath string
	flags := flag.NewFlagSet("sql-monitor", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&opts.logPath, "log", "/tmp/data-service-local.log", "Log file to follow (read-only)")
	flags.IntVar(&opts.port, "port", 8767, "Local HTTP port")
	flags.IntVar(&opts.maxEvents, "max-events", 2000, "In-memory event capacity (1..100000)")
	flags.Int64Var(&opts.initialBytes, "initial-bytes", 2*1024*1024, "Bytes to read from the initial log tail")
	flags.Int64Var(&opts.rawBytes, "raw-bytes", 64<<20, "Byte budget for retained raw SQL (0 disables the byte limit)")
	flags.BoolVar(&opts.stdin, "stdin", false, "Read stdin until EOF; keep the UI alive")
	flags.StringVar(&configPath, "config", "", "Explicit JSON config path (never loaded automatically)")
	var kf k8sFlags
	flags.StringVar(&kf.env, "k8s-env", "", "Kubernetes environment name from config k8s.environments")
	flags.StringVar(&kf.lane, "k8s-lane", "", "Override the lane label value (x2robot.io/lane)")
	flags.BoolVar(&kf.all, "k8s-all", false, "Select every pod with the app label, ignoring lane")
	flags.StringVar(&kf.kubeconfig, "k8s-kubeconfig", "", "Override the kubeconfig path")
	flags.StringVar(&kf.context, "k8s-context", "", "Override the kubeconfig context")
	flags.StringVar(&kf.namespace, "k8s-namespace", "", "Override the namespace (empty uses the context default)")
	flags.StringVar(&kf.app, "k8s-app", "", "Override the app label value")
	flags.StringVar(&kf.container, "k8s-container", "", "Container name for multi-container pods")
	flags.IntVar(&kf.tail, "k8s-tail", k8s.DefaultTail, "Initial lines to backfill (0 disables)")
	flags.StringVar(&kf.kubectl, "k8s-kubectl", "", "kubectl executable path (default: kubectl from PATH)")
	flags.Usage = func() {
		fmt.Fprintln(output, "Local, read-only SQL log monitor. No database access.")
		fmt.Fprintln(output, "Usage: sql-monitor init | sql-monitor [--config PATH] [--stdin | --log PATH] [--port PORT] [--max-events N] [--initial-bytes N]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, errors.New("unexpected positional arguments")
	}
	explicit := make(map[string]bool)
	flags.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	if explicit["stdin"] && explicit["log"] {
		return opts, errors.New("--stdin and --log cannot be supplied together")
	}
	cli := opts
	if explicit["config"] {
		cfg, err := config.Load(configPath)
		if err != nil {
			return opts, err
		}
		if cfg.Source != nil {
			switch cfg.Source.Mode {
			case "file":
				opts.stdin = false
				opts.logPath = cfg.Source.Path
			case "stdin":
				opts.stdin = true
			case "k8s":
				opts.stdin = false
				opts.k8sConfig = cfg.Source.K8s
			}
		}
		if cfg.Port != nil {
			opts.port = *cfg.Port
		}
		if cfg.MaxEvents != nil {
			opts.maxEvents = *cfg.MaxEvents
		}
		if cfg.InitialBytes != nil {
			opts.initialBytes = *cfg.InitialBytes
		}
		if cfg.RawBytes != nil {
			opts.rawBytes = *cfg.RawBytes
		}
		if cfg.Feishu != nil {
			opts.feishu = cfg.Feishu
		}
		if cfg.Analysis != nil {
			opts.analysis = cfg.Analysis
		}
		if cfg.Devops != nil {
			opts.devops = cfg.Devops
		}
		opts.configPath = configPath
		if abs, err := filepath.Abs(configPath); err == nil {
			opts.configPath = abs
		}
	} else if wd, err := os.Getwd(); err == nil {
		opts.configPath = filepath.Join(wd, "sql-monitor.local.json")
		if cfg, err := config.Load(opts.configPath); err == nil && cfg.Devops != nil {
			opts.devops = cfg.Devops
		}
	}
	if explicit["log"] {
		opts.logPath, opts.stdin = cli.logPath, false
	}
	if explicit["stdin"] {
		opts.stdin = cli.stdin
	}
	anyK8s := explicit["k8s-env"] || explicit["k8s-lane"] || explicit["k8s-all"] || explicit["k8s-kubeconfig"] || explicit["k8s-context"] || explicit["k8s-namespace"] || explicit["k8s-app"] || explicit["k8s-container"] || explicit["k8s-tail"] || explicit["k8s-kubectl"]
	if explicit["log"] || explicit["stdin"] {
		if anyK8s {
			return opts, errors.New("--k8s-* cannot be combined with --stdin or --log")
		}
		opts.k8sConfig = nil
	} else if opts.k8sConfig != nil || anyK8s {
		target, err := resolveK8s(opts.k8sConfig, kf, explicit)
		if err != nil {
			return opts, err
		}
		opts.k8s = target
		opts.stdin = false
	}
	if explicit["port"] {
		opts.port = cli.port
	}
	if explicit["max-events"] {
		opts.maxEvents = cli.maxEvents
	}
	if explicit["initial-bytes"] {
		opts.initialBytes = cli.initialBytes
	}
	if explicit["raw-bytes"] {
		opts.rawBytes = cli.rawBytes
	}
	if opts.port < 1 || opts.port > 65535 || opts.maxEvents < 1 || opts.maxEvents > 100000 || opts.initialBytes < 0 || opts.rawBytes < 0 {
		return opts, errors.New("port must be 1..65535, max-events 1..100000, initial-bytes and raw-bytes nonnegative")
	}
	if opts.logPath == "" {
		return opts, errors.New("log path must not be empty")
	}
	return opts, nil
}

// Run executes the sql-monitor runtime: it handles the init/feishu subcommands,
// otherwise it resolves options and serves the UI until ctx is cancelled or the
// HTTP server fails. It returns a process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	stderr = &notificationLog{output: stderr}
	if len(args) > 0 && args[0] == "init" {
		err := config.Init(args[1:], stderr)
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	}
	if len(args) > 0 && args[0] == "feishu" {
		if len(args) > 1 && args[1] == "test" {
			return feishu.TestCommand(args[2:], stdout, stderr)
		}
		fmt.Fprintln(stderr, `unknown feishu subcommand; try "sql-monitor feishu test --config PATH"`)
		return 2
	}
	var flagOutput strings.Builder
	opts, err := ParseOptions(args, &flagOutput)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, flagOutput.String())
		return 0
	}
	if err != nil {
		if flagOutput.Len() > 0 {
			fmt.Fprint(stderr, flagOutput.String())
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 2
	}
	m, err := monitor.New(opts.logPath, opts.maxEvents, opts.initialBytes)
	if err != nil {
		fmt.Fprintln(stderr, "Cannot initialize monitor:", err)
		return 1
	}
	defer m.Close()
	m.SetRawBudget(opts.rawBytes)
	slowMS := 200.0
	if opts.feishu != nil && opts.feishu.SlowMS > 0 {
		slowMS = opts.feishu.SlowMS
	}
	m.SetAggregateStore(monitor.NewAggregateStore(slowMS))
	m.SetTimeSeries(monitor.NewTimeSeriesCollector(slowMS))
	var input *os.File
	if opts.stdin {
		input, err = os.OpenFile("/dev/stdin", os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			fmt.Fprintln(stderr, "Cannot open stdin")
			return 1
		}
		defer input.Close()
		m.SetSource("stdin", "", "reading")
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(opts.port))
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot bind local HTTP server at http://%s/: %v\nIf a monitor is already running, use its existing UI or choose another --port.\n", address, err)
		return 1
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	var notifier *feishu.Notifier
	if opts.feishu != nil && opts.feishu.Enabled {
		notifier = feishu.NewNotifier(*opts.feishu)
		notifier.SetSource(sourceLabel(opts))
	}
	var analyzer *analyze.Analyzer
	if opts.analysis != nil && opts.analysis.Enabled {
		var sender analyze.Sender
		if notifier != nil {
			sender = notifier
		}
		project := ""
		if opts.feishu != nil {
			project = opts.feishu.Project
		}
		analyzer = analyze.New(
			opts.analysis.Command,
			opts.analysis.CWD,
			time.Duration(opts.analysis.TimeoutSeconds)*time.Second,
			project,
			sourceLabel(opts),
			opts.analysis.MaxHotspots,
			sender,
			stderr,
		)
	}
	handler := web.NewHandler(m, port)
	if opts.devops != nil {
		handler.SetDevops(*opts.devops)
	}
	handler.SetConfigPath(opts.configPath)
	handler.SetAnalyzer(analyzer)
	defer handler.Close()
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second,
		MaxHeaderBytes: 64 * 1024, ErrorLog: log.New(io.Discard, "", 0),
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var analysis chan monitor.SQLSummary
	if analyzer != nil {
		analysis = make(chan monitor.SQLSummary, 4)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case summary := <-analysis:
					analyzer.TriggerSummary(summary.Groups)
				}
			}
		}()
	}
	notifyDone := make(chan struct{})
	if opts.feishu != nil && opts.feishu.Enabled {
		slow := opts.feishu.SlowMS
		reports := make(chan monitor.SQLSummary, 8)
		var producers sync.WaitGroup
		start := func(f func()) {
			producers.Add(1)
			go func() { defer producers.Done(); f() }()
		}
		interval := monitor.NewSummaryCollector(slow, time.Now())
		m.SetSummaryCollector(interval)
		start(func() {
			monitor.RunSummaries(ctx, interval, time.Duration(opts.feishu.IntervalSeconds)*time.Second, reports, stderr)
		})
		if thr := opts.feishu.Threshold; thr != nil && thr.Enabled {
			threshold := monitor.NewSummaryCollector(slow, time.Now())
			m.AddSummaryCollector(threshold)
			if analyzer != nil {
				start(func() {
					monitor.RunThresholds(ctx, threshold, time.Duration(thr.WindowSeconds)*time.Second, time.Duration(thr.CooldownSeconds)*time.Second, thr.Errors, thr.Slow, stderr, reports, analysis)
				})
			} else {
				start(func() {
					monitor.RunThresholds(ctx, threshold, time.Duration(thr.WindowSeconds)*time.Second, time.Duration(thr.CooldownSeconds)*time.Second, thr.Errors, thr.Slow, stderr, reports)
				})
			}
		}
		if daily := opts.feishu.Daily; daily != nil && daily.Enabled {
			day := monitor.NewSummaryCollector(slow, time.Now())
			m.AddSummaryCollector(day)
			start(func() { monitor.RunDaily(ctx, day, daily.Hour, daily.Minute, time.Local, reports, stderr) })
		}
		go func() { producers.Wait(); close(reports) }()
		go func() {
			defer close(notifyDone)
			notifier.Run(ctx, reports, stderr)
		}()
		fmt.Fprintf(stderr, "Feishu enabled: %d recipients, %ds ingestion windows, slow >= %.2fms; SQL parameters are not sent. Pending reports are not persisted.\n", len(opts.feishu.Recipients), opts.feishu.IntervalSeconds, opts.feishu.SlowMS)
	} else {
		close(notifyDone)
		if opts.feishu != nil {
			fmt.Fprintln(stderr, "Feishu notifications off: feishu.enabled is false.")
		} else {
			fmt.Fprintln(stderr, "Feishu notifications off: no feishu config (pass --config with feishu.enabled=true).")
		}
	}
	tailDone := make(chan struct{})
	go func() {
		defer close(tailDone)
		if input != nil {
			m.RunInput(ctx, input)
		} else if opts.k8s != nil {
			opts.k8s.Run(ctx, m)
		} else {
			m.Run(ctx)
		}
	}()
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	if opts.stdin {
		fmt.Fprintf(stderr, "SQL Monitor: http://127.0.0.1:%d/\nSource: stdin (UI stays alive after EOF)\nCapacity: %d events (in memory)\n", port, opts.maxEvents)
	} else if opts.k8s != nil {
		source := "k8s " + opts.k8s.Describe()
		if opts.k8s.Environment != "" {
			source += " (env " + opts.k8s.Environment + ")"
		}
		fmt.Fprintf(stderr, "SQL Monitor: http://127.0.0.1:%d/\nSource: %s\nCapacity: %d events (in memory)\n", port, source, opts.maxEvents)
	} else {
		fmt.Fprintf(stdout, "SQL Monitor: http://127.0.0.1:%d/\nLog file: %s\nCapacity: %d events (in memory)\n", port, m.Snapshot().Status.Path, opts.maxEvents)
	}
	code := 0
	select {
	case <-ctx.Done():
	case err := <-serveDone:
		if !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(stderr, "HTTP server stopped:", err)
			code = 1
		}
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
	}
	<-tailDone
	<-notifyDone
	return code
}
