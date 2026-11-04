// Package analyze runs the optional LLM summarization of SQL hotspots. It shells
// out to an external read-only command (typically qoder-mcp/analyze.py), feeds it
// a prompt built from hotspot fingerprints and code locations, and exposes the
// result for the web UI and forwards it to a Sender (Feishu). It never sends raw
// SQL, parameters, or errors to the analyzer.
package analyze

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"local/sql-monitor/internal/monitor"
)

// Sender delivers a plain-text message. feishu.Notifier.SendText implements it.
type Sender interface {
	SendText(ctx context.Context, text string, output io.Writer) error
}

// Status is a point-in-time view of the most recent analysis run. It is safe to
// marshal directly to JSON for /api/analyze/status.
type Status struct {
	Running      bool       `json:"running"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Result       string     `json:"result,omitempty"`
	Error        string     `json:"error,omitempty"`
	SentToFeishu bool       `json:"sent_to_feishu"`
}

// Hotspot is one anonymized SQL fingerprint worth analyzing. It carries no raw
// SQL, parameters, or error text.
type Hotspot struct {
	Fingerprint string
	Source      string
	Count       uint64
	Errors      uint64
	Slow        uint64
	MaxMS       float64
	N1          bool
	N1Max       uint64
	Reason      string
}

// Analyzer runs at most one analysis at a time. Trigger methods return false
// when a run is already in progress or there is nothing to analyze; results
// land in Status and are optionally forwarded to the Sender.
type Analyzer struct {
	command []string
	cwd     string
	timeout time.Duration
	project string
	source  string
	topN    int
	send    Sender
	output  io.Writer

	mu     sync.Mutex
	status Status
	cancel context.CancelFunc
}

func New(command []string, cwd string, timeout time.Duration, project, source string, topN int, send Sender, output io.Writer) *Analyzer {
	if output == nil {
		output = io.Discard
	}
	if topN < 1 {
		topN = 5
	}
	return &Analyzer{
		command: append([]string(nil), command...),
		cwd:     cwd,
		timeout: timeout,
		project: project,
		source:  source,
		topN:    topN,
		send:    send,
		output:  output,
	}
}

// TriggerAggregates starts an analysis from the cumulative aggregate hotspots
// (used by the manual web button). It returns false when idle capacity is
// unavailable or there are no anomalies.
func (a *Analyzer) TriggerAggregates(groups []monitor.SQLAggregate) bool {
	return a.trigger(FromAggregates(groups, a.topN))
}

// TriggerSummary starts an analysis from a window summary's groups (used by the
// threshold alert producer). It returns false when idle capacity is
// unavailable or there are no anomalies.
func (a *Analyzer) TriggerSummary(groups []monitor.SQLSummaryGroup) bool {
	return a.trigger(FromSummary(groups, a.topN))
}

// Status returns a copy of the most recent run's state.
func (a *Analyzer) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

func (a *Analyzer) trigger(hotspots []Hotspot) bool {
	if len(hotspots) == 0 {
		return false
	}
	a.mu.Lock()
	if a.status.Running {
		a.mu.Unlock()
		return false
	}
	started := time.Now()
	a.status = Status{Running: true, StartedAt: &started}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.mu.Unlock()

	prompt := BuildPrompt(hotspots, a.project, a.source)
	go a.run(ctx, prompt)
	return true
}

func (a *Analyzer) run(ctx context.Context, prompt string) {
	result, runErr := a.execute(ctx, prompt)
	sent := false
	if runErr == nil && a.send != nil {
		if err := a.send.SendText(ctx, result, a.output); err == nil {
			sent = true
		}
	}
	a.mu.Lock()
	finished := time.Now()
	a.status.Running = false
	a.status.FinishedAt = &finished
	a.status.Result = result
	a.status.SentToFeishu = sent
	if runErr != nil {
		a.status.Error = runErr.Error()
	} else {
		a.status.Error = ""
	}
	a.cancel = nil
	a.mu.Unlock()
}

// execute runs the configured command with prompt on stdin and returns its
// trimmed stdout. stderr is only surfaced when stdout is empty so diagnostics
// are not lost but normal chatter is dropped.
func (a *Analyzer) execute(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.command[0], a.command[1:]...)
	if a.cwd != "" {
		cmd.Dir = a.cwd
	}
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("分析超时（%s）", a.timeout)
	}
	output := strings.TrimSpace(stdout.String())
	if err != nil {
		if output == "" {
			output = strings.TrimSpace(stderr.String())
		}
		if output == "" {
			output = "分析命令执行失败"
		}
		return output, fmt.Errorf("分析命令执行失败: %w", err)
	}
	if output == "" {
		return "", fmt.Errorf("分析未产生任何输出")
	}
	return output, nil
}

// FromAggregates converts cumulative aggregate groups into ranked hotspots,
// keeping only groups with at least one anomaly signal and at most topN of
// them. monitor.RankHotspots applies the same weighted ordering the UI's
// "热点加权" sort uses.
func FromAggregates(groups []monitor.SQLAggregate, topN int) []Hotspot {
	ranked := monitor.RankHotspots(groups)
	hotspots := make([]Hotspot, 0, topN)
	for _, g := range ranked {
		if g.Errors == 0 && g.Slow == 0 && !g.N1 {
			continue
		}
		hotspots = append(hotspots, Hotspot{
			Fingerprint: g.Fingerprint,
			Source:      g.Source,
			Count:       g.Count,
			Errors:      g.Errors,
			Slow:        g.Slow,
			MaxMS:       g.MaxMS,
			N1:          g.N1,
			N1Max:       g.N1Max,
			Reason:      g.Reason,
		})
		if len(hotspots) >= topN {
			break
		}
	}
	return hotspots
}

// FromSummary converts a window summary's groups (already sorted by impact and
// capped at five) into hotspots. Summary groups carry no N+1 evidence, so N1
// is always false here.
func FromSummary(groups []monitor.SQLSummaryGroup, topN int) []Hotspot {
	hotspots := make([]Hotspot, 0, len(groups))
	for _, g := range groups {
		if g.Errors == 0 && g.Slow == 0 {
			continue
		}
		hotspots = append(hotspots, Hotspot{
			Fingerprint: g.Fingerprint,
			Source:      g.Source,
			Count:       g.Count,
			Errors:      g.Errors,
			Slow:        g.Slow,
			MaxMS:       g.MaxMS,
		})
		if len(hotspots) >= topN {
			break
		}
	}
	return hotspots
}

func latencyText(h Hotspot) string {
	if h.MaxMS <= 0 {
		return "未知"
	}
	return fmt.Sprintf("%g ms", h.MaxMS)
}

// BuildPrompt renders the natural-language prompt the analyzer receives. It
// contains only fingerprints, code locations and aggregate counts — never raw
// SQL, parameters, or error text.
func BuildPrompt(hotspots []Hotspot, project, source string) string {
	var b strings.Builder
	b.WriteString("你是数据库性能分析助手。请只使用只读工具（Read / Grep / Glob）阅读代码，分析下面的 SQL 热点，并输出中文总结。\n\n")
	if project != "" {
		fmt.Fprintf(&b, "项目：%s\n", project)
	}
	if source != "" && source != "local" {
		fmt.Fprintf(&b, "数据来源：%s\n", source)
	}
	b.WriteString("\n热点列表（按严重程度排序；只含模板指纹与代码位置，不含 SQL 正文与参数）：\n")
	for i, h := range hotspots {
		fmt.Fprintf(&b, "%d. 指纹 %s\n   代码位置：%s\n   执行次数 %d | 错误 %d | 慢查询 %d | 峰值耗时 %s\n", i+1, h.Fingerprint, h.Source, h.Count, h.Errors, h.Slow, latencyText(h))
		if h.N1 {
			fmt.Fprintf(&b, "   疑似 N+1（单个 trace 最多重复 %d 次）\n", h.N1Max)
		}
		if h.Reason != "" {
			fmt.Fprintf(&b, "   原因：%s\n", h.Reason)
		}
	}
	b.WriteString("\n请针对每个热点，沿调用链追到业务入口再下结论：\n1. 先用 Read 读代码位置所在的函数，再用 Grep 沿调用链向上追到业务入口（HTTP handler / service 方法 / 定时任务 / 消息消费者等），明确这段 SQL 在哪个业务流程、什么业务场景下被触发、由谁调用；\n2. 若代码位置落在通用 DB 封装层（如 dao / repository / orm 封装 / 连接池），务必继续向上追到真正的业务调用方，不要停留在封装层就下结论；\n3. 基于业务上下文分析可能的性能或正确性问题（如 N+1、缺索引、全表扫描、锁等待、事务过长）；\n4. 给出具体、可执行的优化建议，优先选择不改动业务逻辑的改法。\n\n最后输出一份不超过 800 字的整体总结，优先处理错误与慢查询最严重的热点，并在每个热点的结论中说明你所追到的业务入口与调用链依据。不要修改任何代码，只做只读分析。")
	return b.String()
}
