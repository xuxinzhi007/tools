// Package logger 的 K8s 日志源：通过 kubectl logs -f 跟随集群 pod 输出。
//
// 设计为可选加载 —— 只有用户在前端切到 K8s 源时才启动 kubectl，
// 平时 data-test-platform 完全不碰 kubeconfig 和集群。
package logger

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	k8sAppLabel    = "app"
	k8sLaneLabel   = "x2robot.io/lane"
	k8sDefaultTail = 200
	k8sStderrLimit = 64 * 1024
)

var errK8sStopped = errors.New("k8s source stopped")

// K8sTarget 描述要跟随的 pod 集合。
type K8sTarget struct {
	Environment string // 仅用于展示，如 "dev2"
	Kubectl     string // 默认 "kubectl"
	Kubeconfig  string
	Context     string
	Namespace   string
	App         string // 默认 "data-service"
	Lane        string // 空 = 不过滤 lane，看全部 pod
	Container   string
	Tail        int
}

func (t K8sTarget) withDefaults() K8sTarget {
	if t.Kubectl == "" {
		t.Kubectl = "kubectl"
	}
	if t.App == "" {
		t.App = "data-service"
	}
	if t.Tail <= 0 {
		t.Tail = k8sDefaultTail
	}
	return t
}

// Selector 返回 label 选择器，如 app=data-service,x2robot.io/lane=dev2。
func (t K8sTarget) Selector() string {
	if t.Lane == "" {
		return k8sAppLabel + "=" + t.App
	}
	return k8sAppLabel + "=" + t.App + "," + k8sLaneLabel + "=" + t.Lane
}

// describe 返回人类可读的来源描述。
func (t K8sTarget) describe() string {
	parts := make([]string, 0, 3)
	if t.Namespace != "" {
		parts = append(parts, t.Namespace)
	}
	if t.Lane != "" {
		parts = append(parts, "lane="+t.Lane)
	}
	if t.App != "" {
		parts = append(parts, "app="+t.App)
	}
	return strings.Join(parts, " ")
}

// args 构造 kubectl 参数（不含 kubectl 本身）。
func (t K8sTarget) args(tail int, follow bool) []string {
	args := make([]string, 0, 12)
	if t.Kubeconfig != "" {
		args = append(args, "--kubeconfig", t.Kubeconfig)
	}
	if t.Context != "" {
		args = append(args, "--context", t.Context)
	}
	if t.Namespace != "" {
		args = append(args, "-n", t.Namespace)
	}
	args = append(args, "logs")
	if follow {
		args = append(args, "-f")
	}
	args = append(args, fmt.Sprintf("--tail=%d", tail), "-l", t.Selector())
	if t.Container != "" {
		args = append(args, "--container="+t.Container)
	}
	return args
}

// K8sTailer 跟随 Kubernetes pod 日志并喂给 Hub。
// kubectl 在滚动发布时会被 pod 替换终止，因此断线后自动重连，
// 重连用 tail=0 避免历史行重复回放。
type K8sTailer struct {
	hub     *Hub
	mu      sync.Mutex
	target  K8sTarget
	cancel  context.CancelFunc
	running bool
	state   string // idle / reading / error
	errMsg  string
}

func NewK8sTailer(hub *Hub) *K8sTailer {
	return &K8sTailer{hub: hub, state: "idle"}
}

// Start 开始跟随。重复调用无副作用。
func (k *K8sTailer) Start(t K8sTarget) {
	t = t.withDefaults()

	k.mu.Lock()
	if k.running {
		k.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	k.target = t
	k.cancel = cancel
	k.running = true
	k.state = "reading"
	k.errMsg = ""
	k.mu.Unlock()

	go k.loop(ctx, t)
}

// Stop 停止跟随并杀掉 kubectl。
func (k *K8sTailer) Stop() {
	k.mu.Lock()
	cancel := k.cancel
	k.cancel = nil
	k.running = false
	k.state = "idle"
	k.errMsg = ""
	k.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// Status 返回当前状态，供前端展示。
func (k *K8sTailer) Status() (running bool, state, describe, errMsg string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.running, k.state, k.target.describe(), k.errMsg
}

func (k *K8sTailer) setError(msg string) {
	k.mu.Lock()
	k.state = "error"
	k.errMsg = msg
	k.mu.Unlock()
}

func (k *K8sTailer) setReading() {
	k.mu.Lock()
	k.state = "reading"
	k.errMsg = ""
	k.mu.Unlock()
}

func (k *K8sTailer) loop(ctx context.Context, target K8sTarget) {
	tail := target.Tail
	for {
		if ctx.Err() != nil {
			return
		}
		err := k.stream(ctx, target, tail)
		tail = 0 // 重连不再回放历史

		if ctx.Err() != nil || errors.Is(err, errK8sStopped) {
			return
		}
		if err != nil {
			k.setError(err.Error())
		} else {
			k.setReading()
		}

		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (k *K8sTailer) stream(ctx context.Context, target K8sTarget, tail int) error {
	cmd := exec.CommandContext(ctx, target.Kubectl, target.args(tail, true)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &boundedErr{limit: k8sStderrLimit}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return err
	}

	env := target.Environment
	if env == "" {
		env = "k8s"
	}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	stopped := false
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		// kubectl -l 多 pod 时会给每行加 "pod-name " 前缀，剥掉并单独记录。
		pod, body := splitK8sPrefix(line)
		k.hub.IngestRawLineFromSource(body, env, pod)
		if ctx.Err() != nil {
			stopped = true
			break
		}
	}

	waitErr := cmd.Wait()
	if stopped {
		return errK8sStopped
	}
	if ctx.Err() != nil {
		return nil
	}
	if waitErr != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return errors.New(msg)
		}
		return waitErr
	}
	return nil
}

// splitK8sPrefix 处理 kubectl logs -l 的 "pod-name log line" 前缀。
// 单 pod 时 kubectl 不加前缀，此时原样返回。
func splitK8sPrefix(line string) (pod, body string) {
	idx := strings.IndexByte(line, ' ')
	if idx <= 0 {
		return "", line
	}
	head := line[:idx]
	// pod 名形如 data-service-dev2-78d76496db-mscw6，含连字符且无 JSON 特征
	if !strings.HasPrefix(head, "{") && strings.Count(head, "-") >= 2 && !strings.ContainsAny(head, "{}[]:") {
		return head, line[idx+1:]
	}
	return "", line
}

// boundedErr 只保留 kubectl stderr 的前 limit 字节，避免管道写满阻塞。
type boundedErr struct {
	data  []byte
	limit int
}

func (b *boundedErr) Write(p []byte) (int, error) {
	n := len(p)
	if len(b.data) < b.limit {
		room := b.limit - len(b.data)
		if n > room {
			p = p[:room]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

func (b *boundedErr) String() string {
	return string(b.data)
}
