package logger

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"sync"
	"time"
)

// FileTailer continuously tails a log file and pipes into Hub
type FileTailer struct {
	hub        *Hub
	filePath   string
	cancelFunc context.CancelFunc
	mu         sync.Mutex
	running    bool
}

// NewFileTailer creates a new tailer
func NewFileTailer(hub *Hub, filePath string) *FileTailer {
	return &FileTailer{
		hub:      hub,
		filePath: filePath,
	}
}

// SetFilePath updates target file path
func (t *FileTailer) SetFilePath(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.filePath = path
}

// Start begins tailing the file in the background
func (t *FileTailer) Start() {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancelFunc = cancel
	t.running = true
	t.mu.Unlock()

	go t.tailLoop(ctx)
}

// Stop stops the tailer
func (t *FileTailer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancelFunc != nil {
		t.cancelFunc()
		t.cancelFunc = nil
	}
	t.running = false
}

func (t *FileTailer) tailLoop(ctx context.Context) {
	var currentFile *os.File
	var lastOffset int64 = 0

	for {
		select {
		case <-ctx.Done():
			if currentFile != nil {
				currentFile.Close()
			}
			return
		default:
		}

		t.mu.Lock()
		targetPath := t.filePath
		t.mu.Unlock()

		if targetPath == "" {
			time.Sleep(1 * time.Second)
			continue
		}

		// Check if file exists
		stat, err := os.Stat(targetPath)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		// Handle file truncation or rotation
		if currentFile == nil || stat.Size() < lastOffset {
			if currentFile != nil {
				currentFile.Close()
			}
			f, err := os.Open(targetPath)
			if err != nil {
				time.Sleep(1 * time.Second)
				continue
			}
			currentFile = f
			// If starting first time and file is large, start from last 20KB
			if lastOffset == 0 && stat.Size() > 20480 {
				lastOffset, _ = f.Seek(-20480, io.SeekEnd)
			} else {
				lastOffset, _ = f.Seek(0, io.SeekStart)
			}
		}

		reader := bufio.NewReader(currentFile)
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				t.hub.IngestRawLine(line)
			}
			if err != nil {
				break
			}
		}

		// Record offset
		lastOffset, _ = currentFile.Seek(0, io.SeekCurrent)
		time.Sleep(300 * time.Millisecond)
	}
}

// ProcessRunner manages running the data-service subprocess
type ProcessRunner struct {
	hub        *Hub
	cmd        *exec.Cmd
	mu         sync.Mutex
	running    bool
	cancelFunc context.CancelFunc
}

// NewProcessRunner creates runner
func NewProcessRunner(hub *Hub) *ProcessRunner {
	return &ProcessRunner{hub: hub}
}

// Start launches a command and pipes stdout/stderr into hub
func (r *ProcessRunner) Start(command string, args []string, dir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("process is already running")
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.cancelFunc = cancel
	r.cmd = exec.CommandContext(ctx, command, args...)
	if dir != "" {
		r.cmd.Dir = dir
	}

	stdout, err := r.cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stderr, err := r.cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}

	if err := r.cmd.Start(); err != nil {
		cancel()
		return err
	}
	r.running = true

	pipeReader := func(reader io.Reader) {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			r.hub.IngestRawLine(scanner.Text())
		}
	}

	go pipeReader(stdout)
	go pipeReader(stderr)

	go func() {
		_ = r.cmd.Wait()
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()

	return nil
}

// Stop terminates the running process
func (r *ProcessRunner) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running || r.cancelFunc == nil {
		return nil
	}
	r.cancelFunc()
	r.running = false
	return nil
}

// IsRunning reports process status
func (r *ProcessRunner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// GenerateMockLogs generates realistic sample logs (including fast SQL, slow SQL, error SQL)
func GenerateMockLogs(hub *Hub, count int) {
	sqlTemplates := []struct {
		sql    string
		source string
		slow   bool
	}{
		{
			sql:    "SELECT id, name, status, created_at FROM jobs WHERE tenant_id = 1 AND status = 2 ORDER BY id DESC LIMIT 20",
			source: "internal/data/job.go:134",
			slow:   false,
		},
		{
			sql:    "SELECT e.*, ep.slice_count FROM episodes e LEFT JOIN episode_payloads ep ON e.id = ep.episode_id WHERE e.job_id = 'job_20260924_001' ORDER BY e.created_at DESC",
			source: "internal/data/episode.go:210",
			slow:   false,
		},
		{
			sql:    "SELECT count(*) as total, avg(duration) as avg_dur FROM episodes WHERE tenant_id = 1 GROUP BY model_id, scene_type",
			source: "internal/data/episode_duration_sum.go:88",
			slow:   true, // Aggregation across big table -> Slow SQL!
		},
		{
			sql:    "SELECT * FROM approval_nodes an JOIN review_flows rf ON an.flow_id = rf.id WHERE rf.status IN (1, 2) AND an.reviewer_id = 1001 ORDER BY an.updated_at DESC",
			source: "internal/data/approval.go:95",
			slow:   false,
		},
		{
			sql:    "SELECT j.*, (SELECT count(1) FROM tasks t WHERE t.job_id = j.id) as task_cnt FROM jobs j WHERE j.tenant_id = 1 AND j.created_at > '2026-09-01' ORDER BY j.updated_at DESC",
			source: "internal/data/job_qc_status_filter.go:142",
			slow:   true, // Correlated subquery -> Slow SQL!
		},
		{
			sql:    "UPDATE job_takeovers SET status = 3, updated_at = NOW() WHERE job_id = 'job_20260924_001' AND version = 2",
			source: "internal/data/job_takeover.go:78",
			slow:   false,
		},
		{
			sql:    "INSERT INTO biz_journals (tenant_id, action, target_id, operator_id, created_at) VALUES (1, 'approve_node', 'node_909', 1001, NOW())",
			source: "internal/data/biz_journal.go:45",
			slow:   false,
		},
	}

	generalLogs := []struct {
		level string
		msg   string
		src   string
	}{
		{"INFO", "HTTP server listening on [::]:8000", "internal/server/http.go:45"},
		{"INFO", "gRPC server listening on [::]:9000", "internal/server/grpc.go:38"},
		{"INFO", "MySQL database connected successfully", "internal/data/mysql.go:80"},
		{"INFO", "Redis connection established pool=10", "internal/data/redis.go:50"},
		{"DEBUG", "Cache hit for job_config_template:tpl_1001", "internal/biz/job_config.go:82"},
		{"WARN", "Takeover lock acquisition retrying after 50ms conflict", "internal/biz/takeover.go:167"},
		{"INFO", "Finished batch approval for review nodes count=4", "internal/service/approval.go:89"},
	}

	for i := 0; i < count; i++ {
		now := time.Now()
		traceID := fmt.Sprintf("trace_%04x%04x%04x%04x", rand.Intn(0xffff), rand.Intn(0xffff), rand.Intn(0xffff), rand.Intn(0xffff))

		if rand.Float32() < 0.6 {
			// SQL Log
			item := sqlTemplates[rand.Intn(len(sqlTemplates))]
			var durMs float64
			var elapsed string
			var isSlow bool
			var level string

			if item.slow || rand.Float32() < 0.3 {
				// Slow SQL: 220ms - 850ms
				durMs = 210.0 + rand.Float64()*650.0
				elapsed = fmt.Sprintf("%.2fms", durMs)
				isSlow = true
				level = "WARN"
			} else {
				// Fast SQL: 2.5ms - 85ms
				durMs = 2.0 + rand.Float64()*80.0
				elapsed = fmt.Sprintf("%.2fms", durMs)
				isSlow = false
				level = "DEBUG"
			}

			entry := &LogEntry{
				Timestamp:  now,
				TimeStr:    now.Format("15:04:05.000"),
				Level:      level,
				Message:    "SQL Query",
				Source:     item.source,
				TraceID:    traceID,
				TenantID:   "1",
				UID:        "1001",
				IsSQL:      true,
				IsSlowSQL:  isSlow,
				SQL:        item.sql,
				DurationMs: durMs,
				Elapsed:    elapsed,
				Rows:       int64(rand.Intn(50) + 1),
				Threshold:  "200ms",
			}
			if isSlow {
				entry.Message = "Slow SQL"
			}
			hub.IngestEntry(entry)
		} else {
			// Regular Log
			item := generalLogs[rand.Intn(len(generalLogs))]
			entry := &LogEntry{
				Timestamp: now,
				TimeStr:   now.Format("15:04:05.000"),
				Level:     item.level,
				Message:   item.msg,
				Source:    item.src,
				TraceID:   traceID,
				TenantID:  "1",
				UID:       "1001",
			}
			hub.IngestEntry(entry)
		}
		time.Sleep(15 * time.Millisecond)
	}
}
