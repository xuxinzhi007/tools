package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var portRegex = regexp.MustCompile(`(?m)^\s*addr:\s*["']?(?:[0-9\.]*)?:([0-9]+)["']?`)

// Environment represents a target server environment
type Environment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	BaseURL     string `json:"base_url"`
	RoutePrefix string `json:"route_prefix"`
	Description string `json:"description"`
}

// Config holds runtime configuration for the test platform
type Config struct {
	mu sync.RWMutex

	Port               int           `json:"port"`
	OpenAPIPath        string        `json:"openapi_path"`
	LogFilePath        string        `json:"log_file_path"`
	SlowSQLThreshold   time.Duration `json:"slow_sql_threshold"`
	CurrentEnvID       string        `json:"current_env_id"`
	CurrentRoutePrefix string        `json:"current_route_prefix"`
	CustomBaseURL      string        `json:"custom_base_url"`
	Environments       []Environment `json:"environments"`

	// Default global auth headers
	DefaultTenantID string `json:"default_tenant_id"`
	DefaultUserID   string `json:"default_user_id"`
	DefaultToken    string `json:"default_token"`

	// Gateway headers (x-xr-auth-*)
	EnableXrAuth bool   `json:"enable_xr_auth"`
	XrAuthType   string `json:"xr_auth_type"`
	XrAuthUID    string `json:"xr_auth_uid"`
	XrAuthSub    string `json:"xr_auth_sub"`
	XrAuthTID    string `json:"xr_auth_tid"`
	XrAuthAut    string `json:"xr_auth_aut"`

	// Remote Cloud Log Configuration (Workflow service)
	RemoteLog RemoteLogConfig `json:"remote_log"`

	// K8s Log Source：可选。未配置时不启动 kubectl，完全不碰集群。
	K8s K8sLogConfig `json:"k8s"`

	// ProjectName / ProjectTitle 项目显示名，由 project.json 提供。
	ProjectName  string `json:"project_name"`
	ProjectTitle string `json:"project_title"`
	// ProfileVars 用于展开 header 模板里的 ${var}
	ProfileVars map[string]string `json:"profile_vars,omitempty"`
	// ProfileHeaders 默认注入的请求头
	ProfileHeaders []HeaderRule `json:"profile_headers,omitempty"`
	// DirectHosts 绕过系统代理直连的域名后缀
	DirectHosts []string `json:"direct_hosts,omitempty"`
	// Run 本地启动被测试服务的默认命令
	Run *RunConfig `json:"run,omitempty"`
}

// RunConfig 描述如何本地拉起被测试服务。
type RunConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Dir     string   `json:"dir,omitempty"`
}

// K8sLogConfig 描述 kubectl 跟随集群 pod 日志的参数。
// Enabled 为 false 时前端只能看到选项，启动会报未配置。
type K8sLogConfig struct {
	Enabled    bool   `json:"enabled"`
	Kubectl    string `json:"kubectl"`    // 默认 kubectl
	Kubeconfig string `json:"kubeconfig"` // 留空用默认 kubeconfig
	Context    string `json:"context"`
	App        string `json:"app"`        // 服务名（label selector 值）
	AppLabel   string `json:"app_label"`  // label 键，默认 "app"
	LaneLabel  string `json:"lane_label"` // lane 的 label 键，默认 "lane"
	Container  string `json:"container"`
	Tail       int    `json:"tail"`
	// Environments 列出可选的 lane，如 ["dev1","dev2","test"]
	Environments []K8sEnvConfig `json:"environments"`
}

// K8sEnvConfig 一个可选环境（lane）。
type K8sEnvConfig struct {
	Name      string `json:"name"`      // 展示名，如 "dev2"
	Namespace string `json:"namespace"` // 命名空间/环境标识
	Lane      string `json:"lane"`      // lane 的值；空 = 全部 pod
}

// RemoteLogEnvConfig holds cloud workflow project and namespace/environment for an environment
type RemoteLogEnvConfig struct {
	Environment string   `json:"environment"` // 环境标识，如命名空间名
	Namespace   string   `json:"namespace"`   // alias for backward compatibility
	Service     string   `json:"service"`     // 服务名
	Source      string   `json:"source"`      // default "default"
	ProjectIDs  []string `json:"project_ids"` // optional for backward compatibility
}

// GetEnvironment returns Environment or Namespace
func (e RemoteLogEnvConfig) GetEnvironment() string {
	if e.Environment != "" {
		return e.Environment
	}
	return e.Namespace
}

// RemoteLogConfig holds configuration for polling remote logs from workflow service
type RemoteLogConfig struct {
	WorkflowURL    string                        `json:"workflow_url"`
	AuthToken      string                        `json:"auth_token"`
	Service        string                        `json:"service"`    // 服务名
	Source         string                        `json:"source"`     // default "default"
	QueryMode      string                        `json:"query_mode"` // default "keyword"
	TimeRange      string                        `json:"time_range"` // default "realtime"
	SLSProject     string                        `json:"sls_project"`
	PageSize       int                           `json:"page_size"`
	Realtime       bool                          `json:"realtime"`
	PollIntervalMs int                           `json:"poll_interval_ms"`
	LogSource      string                        `json:"log_source"` // "auto", "local", "dev", "test", "prod"
	Environments   map[string]RemoteLogEnvConfig `json:"environments"`
}

// detectLocalServicePort 从常见的本地配置文件名里探测 HTTP 端口。
// 只扫描通用文件名，不绑定具体项目目录。
func detectLocalServicePort() int {
	candidatePaths := []string{
		"config.yaml",
		"config.local.yaml",
		"configs/config.yaml",
		"configs/config.local.yaml",
		"configs/config.app.yaml",
		"configs/config.app.local.yaml",
	}

	for _, p := range candidatePaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		// Look for http.addr: ...:PORT
		matches := portRegex.FindStringSubmatch(string(data))
		if len(matches) > 1 {
			var port int
			if _, err := fmt.Sscanf(matches[1], "%d", &port); err == nil && port > 0 {
				return port
			}
		}
	}

	return 8005 // Default to 8005 as requested
}

// NewDefaultConfig creates standard configuration
func NewDefaultConfig() *Config {
	localPort := detectLocalServicePort()
	localBaseURL := fmt.Sprintf("http://127.0.0.1:%d", localPort)

	// 这里只给中性默认值。所有具体项目信息（域名、租户、服务名）
	// 由 --project 指向的 profile 文件提供，代码里不出现任何项目硬编码。
	return &Config{
		Port:               9099,
		OpenAPIPath:        "openapi.yaml",
		LogFilePath:        "app.log",
		SlowSQLThreshold:   200 * time.Millisecond,
		CurrentEnvID:       "local",
		CurrentRoutePrefix: "",
		ProjectName:        "service",
		ProjectTitle:       "接口测试与日志 SQL 监控",
		DefaultTenantID:    "",
		DefaultUserID:      "",
		DefaultToken:       "",
		Environments: []Environment{
			{
				ID:          "local",
				Name:        fmt.Sprintf("Local (本地服务 :%d)", localPort),
				BaseURL:     localBaseURL,
				RoutePrefix: "",
				Description: fmt.Sprintf("本地开发运行的服务 (: %d)", localPort),
			},
			{
				ID:          "custom",
				Name:        "Custom (自定义地址)",
				BaseURL:     localBaseURL,
				RoutePrefix: "",
				Description: "自定义指定的目标服务地址与路由前缀",
			},
		},
		RemoteLog: RemoteLogConfig{
			QueryMode:      "keyword",
			TimeRange:      "realtime",
			PageSize:       100,
			Realtime:       true,
			PollIntervalMs: 2000,
			LogSource:      "auto",
			Environments:   map[string]RemoteLogEnvConfig{},
		},
	}
}

// GetBaseURL returns the active target base URL
func (c *Config) GetBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.CurrentEnvID == "custom" && c.CustomBaseURL != "" {
		return c.CustomBaseURL
	}
	for _, env := range c.Environments {
		if env.ID == c.CurrentEnvID {
			return env.BaseURL
		}
	}
	return "http://127.0.0.1:8005"
}

// GetRoutePrefix returns the active route prefix (e.g. "/core")
func (c *Config) GetRoutePrefix() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.CurrentRoutePrefix != "" {
		return c.CurrentRoutePrefix
	}
	for _, env := range c.Environments {
		if env.ID == c.CurrentEnvID {
			return env.RoutePrefix
		}
	}
	return ""
}

// SetCurrentEnv updates the active environment
func (c *Config) SetCurrentEnv(envID string, customURL string, routePrefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.CurrentEnvID = envID
	if customURL != "" {
		c.CustomBaseURL = customURL
	}
	if routePrefix != "" {
		c.CurrentRoutePrefix = routePrefix
	} else {
		for _, env := range c.Environments {
			if env.ID == envID {
				c.CurrentRoutePrefix = env.RoutePrefix
				break
			}
		}
	}
}

// SetSlowSQLThreshold updates the slow SQL threshold
func (c *Config) SetSlowSQLThreshold(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SlowSQLThreshold = d
}

// GetSlowSQLThreshold returns the slow SQL threshold
func (c *Config) GetSlowSQLThreshold() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.SlowSQLThreshold
}

// GetRemoteLogConfig returns a copy of remote log config
func (c *Config) GetRemoteLogConfig() RemoteLogConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.RemoteLog
}

// SetRemoteLogConfig updates remote log config
func (c *Config) SetRemoteLogConfig(rl RemoteLogConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.RemoteLog = rl
}

// UpdateRemoteLogConfig applies partial or full updates
func (c *Config) UpdateRemoteLogConfig(update RemoteLogConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if update.WorkflowURL != "" {
		c.RemoteLog.WorkflowURL = update.WorkflowURL
	}
	if update.AuthToken != "" {
		c.RemoteLog.AuthToken = update.AuthToken
	}
	c.RemoteLog.SLSProject = update.SLSProject
	if update.PageSize > 0 {
		c.RemoteLog.PageSize = update.PageSize
	}
	if update.PollIntervalMs > 0 {
		c.RemoteLog.PollIntervalMs = update.PollIntervalMs
	}
	if update.LogSource != "" {
		c.RemoteLog.LogSource = update.LogSource
	}
	c.RemoteLog.Realtime = update.Realtime
	if update.Environments != nil {
		if c.RemoteLog.Environments == nil {
			c.RemoteLog.Environments = make(map[string]RemoteLogEnvConfig)
		}
		for k, v := range update.Environments {
			c.RemoteLog.Environments[k] = v
		}
	}
}

// GetActiveRemoteLogTarget determines if remote polling is active and for which env
func (c *Config) GetActiveRemoteLogTarget() (string, RemoteLogEnvConfig, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	source := c.RemoteLog.LogSource
	if source == "" || source == "auto" {
		source = c.CurrentEnvID
	}

	if source == "dev" || source == "test" || source == "prod" {
		if envCfg, ok := c.RemoteLog.Environments[source]; ok {
			return source, envCfg, true
		}
	}
	return "local", RemoteLogEnvConfig{}, false
}

// LoadK8sConfig 从 JSON 文件读取 K8s 日志源配置。
// 文件不存在或 enabled=false 时保持禁用，调用方不应因此启动 kubectl。
func LoadK8sConfig(path string, out *K8sLogConfig) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	// App 必须由配置提供，避免默认连到某个具体服务

	if out.Kubectl == "" {
		out.Kubectl = "kubectl"
	}
	if out.Tail <= 0 {
		out.Tail = 200
	}
	return nil
}

// HeaderRule 一条默认请求头规则。key 与 value 支持 ${var} 展开，
// 变量来自 Profile.Vars，这样不同项目的网关鉴权头可以完全配置化。
type HeaderRule struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Desc  string `json:"desc,omitempty"`
	// Secret 为 true 时前端不回显真实值，只显示掩码
	Secret bool `json:"secret,omitempty"`
}

// ProjectProfile 描述被测试项目，所有项目相关信息集中于此。
// 代码里不留任何具体项目名、域名或账号。
type ProjectProfile struct {
	// Name 项目显示名
	Name string `json:"name"`
	// Title 页面标题
	Title string `json:"title,omitempty"`
	// Vars 变量表，HeaderRule.Value 里的 ${xxx} 从这里展开
	Vars map[string]string `json:"vars,omitempty"`
	// Environments 目标环境列表
	Environments []Environment `json:"environments,omitempty"`
	// Headers 默认注入的请求头（带 ${var} 占位）
	Headers []HeaderRule `json:"headers,omitempty"`
	// RemoteLog 云端日志查询配置
	RemoteLog *RemoteLogConfig `json:"remote_log,omitempty"`
	// DirectHosts 需要绕过系统代理直连的域名后缀，如 [".example.com", ".example.cn"]
	DirectHosts []string `json:"direct_hosts,omitempty"`
	// Run 本地启动被测试服务的默认命令
	Run *RunConfig `json:"run,omitempty"`
}

// ExpandVars 展开 ${name} 占位符。未定义的变量保留原文，便于前端提示。
func (p *ProjectProfile) ExpandVars(s string) string {
	if len(p.Vars) == 0 || !strings.Contains(s, "${") {
		return s
	}
	return placeholderVars.ReplaceAllStringFunc(s, func(m string) string {
		key := m[2 : len(m)-1]
		if v, ok := p.Vars[key]; ok {
			return v
		}
		return m
	})
}

var placeholderVars = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// LoadProfile 从 JSON 文件读取项目配置。文件不存在时返回 nil 且不报错。
func LoadProfile(path string) (*ProjectProfile, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var p ProjectProfile
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &p, nil
}

// ApplyProfile 把项目配置合并进运行配置。
// 未在 profile 里声明的字段保留原有默认值，因此 profile 可以是部分覆盖。
func (c *Config) ApplyProfile(p *ProjectProfile) {
	if p == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if p.Name != "" {
		c.ProjectName = p.Name
	}
	if p.Title != "" {
		c.ProjectTitle = p.Title
	}
	c.ProfileVars = p.Vars
	if len(p.Environments) > 0 {
		c.Environments = p.Environments
	}
	c.ProfileHeaders = p.Headers
	c.DirectHosts = p.DirectHosts
	c.Run = p.Run
	if p.RemoteLog != nil {
		c.RemoteLog = *p.RemoteLog
	}
}
