package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type State string

const (
	Stopped  State = "stopped"
	Starting State = "starting"
	Running  State = "running"
	External State = "external" // 端口被别的进程占着，不是我们起的
	Failed   State = "failed"
)

type Proc struct {
	Tool    Tool
	State   State
	PID     int
	Err     string
	StartAt time.Time
	Lines   []string // 尾部日志，环形保留

	cmd    *exec.Cmd
	cancel context.CancelFunc
	logf   *os.File
	mu     sync.Mutex
}

type Manager struct {
	cfg  *config
	mu   sync.Mutex
	proc map[string]*Proc
	pidf string
	ownf string // 记录自己拉起的子进程 pid，用于重启后认领孤儿
}

func newManager(c *config) *Manager {
	m := &Manager{
		cfg:  c,
		proc: map[string]*Proc{},
		pidf: filepath.Join(c.rootDir, "pid"),
		ownf: filepath.Join(c.rootDir, "children.json"),
	}
	for _, t := range c.tools.Tools {
		m.proc[t.ID] = &Proc{Tool: t, State: Stopped, Lines: []string{}}
	}
	return m
}

// 子进程 PGID 落盘。toolbox 被 kill -9 时用它认领自己留下的孤儿，
// 而不是一律当成“用户手动启动”而不敢动。
func (m *Manager) saveChildren() {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]int{}
	for id, p := range m.proc {
		p.mu.Lock()
		if p.cmd != nil && p.cmd.Process != nil {
			out[id] = p.cmd.Process.Pid
		}
		p.mu.Unlock()
	}
	raw, _ := json.Marshal(out)
	_ = os.WriteFile(m.ownf, raw, 0o600)
}

func (m *Manager) loadChildren() map[string]int {
	out := map[string]int{}
	raw, err := os.ReadFile(m.ownf)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func (m *Manager) writePID() {
	_ = os.WriteFile(m.pidf, []byte(strconv.Itoa(os.Getpid())), 0o600)
}

// 上一个 toolbox 被 kill -9 强杀时，子进程会变成孤儿继续占端口。
// 启动时如果发现有孤儿在监听、且不是当前用户手动起的，直接收掉。
func (m *Manager) markListening() (adopted, external int) {
	own := m.loadChildren()
	for _, t := range m.cfg.tools.Tools {
		if t.Port == 0 || !portOpen(t.Port) {
			continue
		}
		p := m.proc[t.ID]
		p.mu.Lock()
		// 只有 children.json 记录过、且进程组还活着的，才是自己上次留下的孤儿。
		if pgid, ok := own[t.ID]; ok && syscall.Kill(-pgid, 0) == nil {
			p.State = Running
			p.PID = pgid
			p.StartAt = time.Now()
			adopted++
		} else {
			p.State = External
			external++
		}
		p.mu.Unlock()
	}
	return adopted, external
}

func (m *Manager) get(id string) *Proc {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.proc[id]
}

func (m *Manager) all() []*Proc {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Proc, 0, len(m.proc))
	for _, t := range m.cfg.tools.Tools {
		out = append(out, m.proc[t.ID])
	}
	return out
}

// probe 每次查状态都探一次端口，这样手动 make run 起的进程也能被认出来。
func (m *Manager) probe(p *Proc) State {
	p.mu.Lock()
	cur := p.State
	p.mu.Unlock()

	if cur == Starting || cur == Failed {
		return cur
	}

	if !portOpen(p.Tool.Port) {
		if cur == Running || cur == External {
			p.mu.Lock()
			p.State = Stopped
			p.PID = 0
			p.mu.Unlock()
			return Stopped
		}
		return Stopped
	}

	if cur == Running {
		return Running
	}
	// 端口通但不是我们起的
	p.mu.Lock()
	if p.State == Stopped {
		p.State = External
	}
	got := p.State
	p.mu.Unlock()
	return got
}

func portOpen(port int) bool {
	if port == 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (m *Manager) start(id string) error {
	p := m.get(id)
	if p == nil {
		return fmt.Errorf("unknown tool: %s", id)
	}
	if len(p.Tool.Cmd) == 0 {
		return fmt.Errorf("%s 没有启动命令（%s 分组只做展示）", p.Tool.Name, p.Tool.Group)
	}

	if st := m.probe(p); st == Running || st == External || st == Starting {
		return fmt.Errorf("%s 已在运行", p.Tool.Name)
	}

	if p.Tool.Port != 0 && portOpen(p.Tool.Port) {
		return fmt.Errorf("端口 %d 已被占用，先停掉那个进程", p.Tool.Port)
	}

	dir := m.cfg.toolDir(p.Tool)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("目录不存在: %s", dir)
	}

	args := make([]string, len(p.Tool.Cmd))
	for i, a := range p.Tool.Cmd {
		args[i] = m.cfg.resolve(a)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(),
		"TOOLBOX=1",
		"FEISHU_APP_ID="+m.cfg.secrets["secrets.feishu.app_id"],
		"FEISHU_APP_SECRET="+m.cfg.secrets["secrets.feishu.app_secret"],
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("启动失败: %w", err)
	}

	p.mu.Lock()
	p.cmd = cmd
	p.cancel = cancel
	p.State = Starting
	p.PID = cmd.Process.Pid
	p.Err = ""
	p.StartAt = time.Now()
	p.Lines = p.Lines[:0]
	p.mu.Unlock()

	go m.pump(p, stdout)
	m.saveChildren()

	go func() {
		err := cmd.Wait()
		cancel()
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.State == Stopped {
			return
		}
		if err != nil && p.State != Stopped {
			p.State = Failed
			p.Err = err.Error()
		} else {
			p.State = Stopped
		}
		p.PID = 0
		go m.saveChildren()
	}()

	// 等端口起来（最多 30s，go run 首次编译比较慢）
	if p.Tool.Port != 0 {
		go m.waitUp(p)
	} else {
		p.mu.Lock()
		p.State = Running
		p.mu.Unlock()
	}
	return nil
}

func (m *Manager) waitUp(p *Proc) {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if portOpen(p.Tool.Port) {
			p.mu.Lock()
			if p.State == Starting {
				p.State = Running
			}
			p.mu.Unlock()
			return
		}
		p.mu.Lock()
		st := p.State
		p.mu.Unlock()
		if st == Failed || st == Stopped {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	p.mu.Lock()
	if p.State == Starting {
		p.State = Failed
		p.Err = fmt.Sprintf("端口 %d 在 60s 内未就绪，看日志", p.Tool.Port)
	}
	p.mu.Unlock()
}

func (m *Manager) pump(p *Proc, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		p.mu.Lock()
		p.Lines = append(p.Lines, line)
		if len(p.Lines) > 400 {
			p.Lines = p.Lines[len(p.Lines)-400:]
		}
		p.mu.Unlock()
	}
}

func (m *Manager) stop(id string) error {
	p := m.get(id)
	if p == nil {
		return fmt.Errorf("unknown tool: %s", id)
	}

	p.mu.Lock()
	cmd, cancel, st, pid := p.cmd, p.cancel, p.State, p.PID
	p.State = Stopped
	p.mu.Unlock()

	if st == External {
		return fmt.Errorf("%s 不是工具箱启动的，请手动关闭", p.Tool.Name)
	}

	// 认领来的孤儿没有 cmd，但有 pid（启动时就是它的进程组 ID）。
	pgid := 0
	if cmd != nil && cmd.Process != nil {
		pgid = cmd.Process.Pid
	} else if pid > 0 {
		pgid = pid
	}
	if pgid == 0 {
		return nil
	}

	// 整个进程组一起收，go run 会 fork 出真实子进程
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			if syscall.Kill(-pgid, 0) != nil {
				close(done)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		close(done)
	}()
	<-done

	if cancel != nil {
		cancel()
	}
	m.saveChildren()
	return nil
}

// 清理占着工具端口的残留进程（上次 toolbox 被强杀留下的孤儿）。
// 只杀监听在已配置端口上的本机进程，先 SIGTERM，不退再 SIGKILL。
func (m *Manager) killExternal(id string) error {
	p := m.get(id)
	if p == nil {
		return fmt.Errorf("unknown tool: %s", id)
	}
	if p.Tool.Port == 0 {
		return fmt.Errorf("%s 没有端口，无法清理", p.Tool.Name)
	}
	pids, err := portPIDs(p.Tool.Port)
	if err != nil {
		return err
	}
	if len(pids) == 0 {
		return fmt.Errorf("端口 %d 没有监听进程", p.Tool.Port)
	}
	for _, pid := range pids {
		if pid == os.Getpid() {
			continue
		}
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			continue
		}
		for i := 0; i < 30; i++ {
			if syscall.Kill(pid, 0) != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if syscall.Kill(pid, 0) == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	time.Sleep(300 * time.Millisecond)
	if portOpen(p.Tool.Port) {
		return fmt.Errorf("端口 %d 仍被占用", p.Tool.Port)
	}
	p.mu.Lock()
	p.State = Stopped
	p.mu.Unlock()
	return nil
}

// 用 lsof 找端口的监听 PID。macOS / Linux 都有。
func portPIDs(port int) ([]int, error) {
	out, err := exec.Command("lsof", "-nP", "-t", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN").Output()
	if err != nil {
		return nil, nil // lsof 无结果时返回 exit 1
	}
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(f); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

func (m *Manager) stopAll() {
	var wg sync.WaitGroup
	for _, p := range m.all() {
		p.mu.Lock()
		running := p.cmd != nil && p.cmd.Process != nil
		p.mu.Unlock()
		if !running {
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_ = m.stop(id)
		}(p.Tool.ID)
	}
	wg.Wait()
	_ = os.Remove(m.pidf)
}

func (m *Manager) autostart() {
	for _, t := range m.cfg.tools.Tools {
		if t.Autostart && len(t.Cmd) > 0 {
			_ = m.start(t.ID)
		}
	}
}

func (m *Manager) tail(id string, n int) []string {
	p := m.get(id)
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if n <= 0 || n > len(p.Lines) {
		n = len(p.Lines)
	}
	out := make([]string, n)
	copy(out, p.Lines[len(p.Lines)-n:])
	return out
}

func (m *Manager) logPath(id string) string {
	return filepath.Join(m.cfg.rootDir, "logs", id+".log")
}

func httpOK(url string) bool {
	c := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode < 500
}
