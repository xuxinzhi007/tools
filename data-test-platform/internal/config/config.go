package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
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
}

// K8sLogConfig 描述 kubectl 跟随集群 pod 日志的参数。
// Enabled 为 false 时前端只能看到选项，启动会报未配置。
type K8sLogConfig struct {
	Enabled    bool   `json:"enabled"`
	Kubectl    string `json:"kubectl"`    // 默认 kubectl
	Kubeconfig string `json:"kubeconfig"` // 留空用默认 kubeconfig
	Context    string `json:"context"`
	App        string `json:"app"` // 默认 data-service
	Container  string `json:"container"`
	Tail       int    `json:"tail"`
	// Environments 列出可选的 lane，如 ["dev1","dev2","test"]
	Environments []K8sEnvConfig `json:"environments"`
}

// K8sEnvConfig 一个可选环境（lane）。
type K8sEnvConfig struct {
	Name      string `json:"name"`      // 展示名，如 "dev2"
	Namespace string `json:"namespace"` // 如 ns-cloud-dev
	Lane      string `json:"lane"`      // x2robot.io/lane 的值；空 = 全部 pod
}

// RemoteLogEnvConfig holds cloud workflow project and namespace/environment for an environment
type RemoteLogEnvConfig struct {
	Environment string   `json:"environment"` // e.g. "ns-cloud-dev", "ns-cloud-test", "ns-cloud-prod"
	Namespace   string   `json:"namespace"`   // alias for backward compatibility
	Service     string   `json:"service"`     // default "data-service"
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
	Service        string                        `json:"service"`    // default "data-service"
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

// detectLocalServicePort reads config.app.local.yaml or config.app.yaml to find the HTTP port
func detectLocalServicePort() int {
	candidatePaths := []string{
		"../data-service/configs/config.app.local.yaml",
		"../data-service/configs/config.app.yaml",
		"configs/config.app.local.yaml",
		"configs/config.app.yaml",
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

	return &Config{
		Port:               9099,
		OpenAPIPath:        "../data-service/openapi.yaml",
		LogFilePath:        "../data-service/logs/app.log",
		SlowSQLThreshold:   200 * time.Millisecond,
		CurrentEnvID:       "local",
		CurrentRoutePrefix: "",
		Environments: []Environment{
			{
				ID:          "local",
				Name:        fmt.Sprintf("Local (本地服务 :%d)", localPort),
				BaseURL:     localBaseURL,
				RoutePrefix: "",
				Description: fmt.Sprintf("本地开发运行的 data-service (: %d)", localPort),
			},
			{
				ID:          "dev",
				Name:        "Dev (开发环境)",
				BaseURL:     "https://dev-api.x2robot.com",
				RoutePrefix: "/core",
				Description: "开发环境网关路由 (https://dev-api.x2robot.com/core)",
			},
			{
				ID:          "test",
				Name:        "Test (测试环境)",
				BaseURL:     "https://test-api.x2robot.com",
				RoutePrefix: "/core",
				Description: "测试环境网关路由 (https://test-api.x2robot.com/core)",
			},
			{
				ID:          "custom",
				Name:        "Custom (自定义地址)",
				BaseURL:     localBaseURL,
				RoutePrefix: "",
				Description: "自定义指定的目标服务地址与路由前缀",
			},
		},
		DefaultTenantID: "888888",
		DefaultUserID:   "563",
		DefaultToken:    "",

		EnableXrAuth: true,
		XrAuthType:   "jwt",
		XrAuthUID:    "563",
		XrAuthSub:    "pengcong@x2robot",
		XrAuthTID:    "888888",
		XrAuthAut:    "SYS_ADMIN",

		RemoteLog: RemoteLogConfig{
			WorkflowURL:    "https://devops.x2robot.com/api/v1/observability/logs/query",
			AuthToken:      "",
			Service:        "data-service",
			Source:         "default",
			QueryMode:      "keyword",
			TimeRange:      "realtime",
			SLSProject:     "",
			PageSize:       100,
			Realtime:       true,
			PollIntervalMs: 2000,
			LogSource:      "auto",
			Environments: map[string]RemoteLogEnvConfig{
				"dev": {
					Environment: "ns-cloud-dev",
					Namespace:   "ns-cloud-dev",
					Service:     "data-service",
					Source:      "default",
				},
				"test": {
					Environment: "ns-cloud-test",
					Namespace:   "ns-cloud-test",
					Service:     "data-service",
					Source:      "default",
				},
				"prod": {
					Environment: "ns-cloud-prod",
					Namespace:   "ns-cloud-prod",
					Service:     "data-service",
					Source:      "default",
				},
			},
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
	if out.App == "" {
		out.App = "data-service"
	}
	if out.Kubectl == "" {
		out.Kubectl = "kubectl"
	}
	if out.Tail <= 0 {
		out.Tail = 200
	}
	return nil
}
