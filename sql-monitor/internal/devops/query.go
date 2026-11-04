package devops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"local/sql-monitor/internal/config"
	"local/sql-monitor/internal/monitor"
)

const (
	defaultURL         = "https://devops.x2robot.com/api/v1/observability/logs/query"
	defaultEnvironment = "ns-cloud-test"
	defaultService     = "data-service"
	pageSize           = 100
	seenLimit          = 2000
)

var errNoToken = errors.New("missing token")

// Poller reads the devops log query API into its own live store. Tokens come
// from the environment and are never returned to the browser.
type Poller struct {
	url         string
	token       string
	environment string
	service     string
	client      *http.Client
	logs        *monitor.Monitor

	mu      sync.Mutex
	active  bool
	started bool
	errText string
	seen    map[[32]byte]struct{}
	order   [][32]byte
}

func New(cfg config.DevopsConfig) *Poller {
	url := cfg.URL
	if url == "" {
		url = defaultURL
	}
	env := cfg.Environment
	if env == "" {
		env = defaultEnvironment
	}
	service := cfg.Service
	if service == "" {
		service = defaultService
	}
	return &Poller{
		url:         url,
		token:       strings.TrimPrefix(strings.TrimSpace(cfg.Token), "Bearer "),
		environment: env,
		service:     service,
		client:      &http.Client{Timeout: 8 * time.Second},
		logs:        newRemoteMonitor(env, service),
		seen:        make(map[[32]byte]struct{}),
	}
}

func Disabled() *Poller {
	return &Poller{
		url:         defaultURL,
		environment: defaultEnvironment,
		service:     defaultService,
		client:      &http.Client{Timeout: 8 * time.Second},
		logs:        newRemoteMonitor(defaultEnvironment, defaultService),
		errText:     "线上日志未配置：点「线上」后在页面填写令牌",
	}
}

func newRemoteMonitor(env, service string) *monitor.Monitor {
	m, err := monitor.New("/dev/null", 2000, 0)
	if err != nil {
		m, _ = monitor.New(".", 2000, 0)
	}
	m.SetSource("remote", env+" "+service, "reading")
	m.SetAggregateStore(monitor.NewAggregateStore(200))
	m.SetTimeSeries(monitor.NewTimeSeriesCollector(200))
	return m
}

func (p *Poller) Active() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active
}

func (p *Poller) SetActive(on bool) {
	p.mu.Lock()
	p.active = on
	token := p.token
	logs := p.logs
	errText := p.errText
	p.mu.Unlock()
	if !on || logs == nil {
		return
	}
	if token == "" {
		if errText == "" {
			errText = "线上日志未配置：点「线上」后在页面填写令牌"
		}
		logs.SetInputState("error", errText)
		return
	}
	p.ensure()
}

func (p *Poller) Logs() *monitor.Monitor {
	return p.logs
}

func (p *Poller) Error() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.errText
}

// Configure replaces the query target. An empty token keeps the one already
// stored in this process.
func (p *Poller) Configure(cfg config.DevopsConfig) {
	_ = p.Apply(cfg)
}

// Apply stores the query target. An empty token, environment, or service keeps
// the current value. The token is not returned to the page.
func (p *Poller) Apply(cfg config.DevopsConfig) error {
	p.mu.Lock()
	url := cfg.URL
	if url == "" {
		url = p.url
	}
	if url == "" {
		url = defaultURL
	}
	env := strings.TrimSpace(cfg.Environment)
	if env == "" {
		env = p.environment
	}
	if env == "" {
		env = defaultEnvironment
	} else {
		env = config.ResolveDevopsEnvironment(env)
	}
	service := strings.TrimSpace(cfg.Service)
	if service == "" {
		service = p.service
	}
	if service == "" {
		service = defaultService
	}
	token := strings.TrimPrefix(strings.TrimSpace(cfg.Token), "Bearer ")
	if token == "" {
		token = p.token
	}
	if token == "" {
		p.mu.Unlock()
		return errNoToken
	}
	p.url = url
	p.token = token
	p.environment = env
	p.service = service
	p.errText = ""
	p.seen = make(map[[32]byte]struct{})
	p.order = nil
	p.logs = newRemoteMonitor(env, service)
	p.active = true
	start := !p.started
	if start {
		p.started = true
	}
	p.mu.Unlock()
	if start {
		go p.loop()
	}
	return nil
}

func (p *Poller) Export() config.DevopsConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return config.DevopsConfig{URL: p.url, Token: p.token, Environment: p.environment, Service: p.service}
}

func (p *Poller) SetURL(url string) {
	p.mu.Lock()
	p.url = url
	p.mu.Unlock()
}

func (p *Poller) Settings() (tokenSet bool, environment, service string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.token != "", p.environment, p.service
}

type view struct {
	monitor.LiveSnapshot
	RemoteError string `json:"remote_error,omitempty"`
}

func (p *Poller) View() view {
	p.ensure()
	p.mu.Lock()
	errText := p.errText
	p.mu.Unlock()
	out := view{LiveSnapshot: p.logs.LiveSnapshot()}
	if errText != "" {
		out.RemoteError = errText
	}
	return out
}

func (p *Poller) ensure() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || p.token == "" {
		return
	}
	p.started = true
	go p.loop()
}

func (p *Poller) loop() {
	p.fetch()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		p.fetch()
	}
}

func (p *Poller) fetch() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	lines, err := p.query(ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		if errors.Is(err, errNoToken) {
			p.errText = "线上日志未配置：点「线上」后在页面填写令牌"
		} else {
			p.errText = "线上日志查询失败"
		}
		if p.logs != nil {
			p.logs.SetInputState("error", p.errText)
		}
		return
	}
	p.errText = ""
	if p.logs != nil {
		p.logs.SetInputState("reading", "")
	}
	for _, line := range lines {
		sum := sha256.Sum256([]byte(line))
		if _, ok := p.seen[sum]; ok {
			continue
		}
		p.seen[sum] = struct{}{}
		p.order = append(p.order, sum)
		if len(p.order) > seenLimit {
			delete(p.seen, p.order[0])
			p.order = p.order[1:]
		}
		p.logs.Consume([]byte(line + "\n"))
	}
}

func (p *Poller) query(ctx context.Context) ([]string, error) {
	p.mu.Lock()
	url, token, environment, service := p.url, p.token, p.environment, p.service
	p.mu.Unlock()
	if token == "" {
		return nil, errNoToken
	}
	body, err := json.Marshal(map[string]any{
		"source":      "default",
		"environment": environment,
		"service":     service,
		"queryMode":   "keyword",
		"page":        1,
		"pageSize":    pageSize,
		"timeRange":   "realtime",
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	lines, err := Lines(payload)
	if err != nil {
		return nil, err
	}
	// Realtime pages are newest-first. Ingest oldest-first so a request's
	// req, SQL, and resp stay in order.
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines, nil
}

// Lines pulls log text out of the query response. A line is either a JSON log
// object or a string field that already holds that object.
func Lines(payload []byte) ([]string, error) {
	var root any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, err
	}
	var lines []string
	collect(&lines, root, 0)
	if lines == nil {
		lines = []string{}
	}
	return lines, nil
}

func collect(lines *[]string, value any, depth int) {
	if depth > 6 {
		return
	}
	switch v := value.(type) {
	case string:
		text := strings.TrimSpace(v)
		if strings.HasPrefix(text, "{") && strings.Contains(text, `"msg"`) {
			*lines = append(*lines, text)
		}
	case map[string]any:
		if _, ok := v["msg"]; ok {
			if encoded, err := json.Marshal(v); err == nil {
				*lines = append(*lines, string(encoded))
			}
			return
		}
		for _, key := range []string{"content", "message", "log", "raw", "line", "body"} {
			if text, ok := v[key].(string); ok {
				trimmed := strings.TrimSpace(text)
				if strings.HasPrefix(trimmed, "{") {
					*lines = append(*lines, trimmed)
					return
				}
			}
		}
		for _, key := range []string{"data", "logs", "items", "records", "list", "rows"} {
			if child, ok := v[key]; ok {
				collect(lines, child, depth+1)
			}
		}
	case []any:
		for _, item := range v {
			collect(lines, item, depth+1)
		}
	}
}
