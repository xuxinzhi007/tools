package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Workspace string            `yaml:"workspace"`
	Listen    Listen            `yaml:"listen"`
	Secrets   map[string]any    `yaml:"secrets"`
	Paths     map[string]string `yaml:"paths"`
}

type Listen struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type ToolsFile struct {
	ListenHost string `yaml:"listen_host"`
	Tools      []Tool `yaml:"tools"`
}

type Tool struct {
	ID         string   `yaml:"id"`
	Name       string   `yaml:"name"`
	Desc       string   `yaml:"desc"`
	Group      string   `yaml:"group"`
	Dir        string   `yaml:"dir"`
	Cmd        []string `yaml:"cmd"`
	Port       int      `yaml:"port"`
	URL        string   `yaml:"url"`
	Health     string   `yaml:"health"`
	Log        string   `yaml:"log"`
	Doc        string   `yaml:"doc"`
	Transport  string   `yaml:"transport"`
	Needs      []string `yaml:"needs"`
	Autostart  bool     `yaml:"autostart"`
	Deprecated bool     `yaml:"deprecated"`
	Source     string   `yaml:"source"`
	Endpoint   string   `yaml:"endpoint"`
}

var placeholder = regexp.MustCompile(`\$\{([a-zA-Z0-9_.]+)\}`)

type config struct {
	rootDir  string
	toolsDir string
	cfg      Config
	tools    ToolsFile

	secrets map[string]string
	paths   map[string]string
	missing map[string][]string
}

func load() (*config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(home, ".toolbox")

	c := &config{
		rootDir: base,
		secrets: map[string]string{},
		paths:   map[string]string{},
		missing: map[string][]string{},
	}

	raw, err := os.ReadFile(filepath.Join(base, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read %s/config.yaml: %w", base, err)
	}
	if err := yaml.Unmarshal(raw, &c.cfg); err != nil {
		return nil, fmt.Errorf("parse config.yaml: %w", err)
	}

	raw, err = os.ReadFile(filepath.Join(base, "tools.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read %s/tools.yaml: %w", base, err)
	}
	if err := yaml.Unmarshal(raw, &c.tools); err != nil {
		return nil, fmt.Errorf("parse tools.yaml: %w", err)
	}

	for k, v := range c.cfg.Paths {
		c.paths[k] = os.ExpandEnv(v)
	}
	c.toolsDir = filepath.Join(c.cfg.Workspace, "tools")
	c.paths["tools"] = c.toolsDir
	c.paths["workspace"] = c.cfg.Workspace
	c.paths["home"] = home

	flatten("secrets", c.cfg.Secrets, c.secrets)
	c.recordMissing()

	if c.cfg.Listen.Port == 0 {
		c.cfg.Listen.Port = 7700
	}
	if c.cfg.Listen.Host == "" {
		c.cfg.Listen.Host = "127.0.0.1"
	}
	if c.tools.ListenHost == "" {
		c.tools.ListenHost = c.cfg.Listen.Host
	}
	return c, nil
}

func flatten(prefix string, in map[string]any, out map[string]string) {
	for k, v := range in {
		key := prefix + "." + k
		switch t := v.(type) {
		case map[string]any:
			flatten(key, t, out)
		case string:
			out[key] = t
		case nil:
			out[key] = ""
		default:
			out[key] = fmt.Sprint(t)
		}
	}
}

func (c *config) recordMissing() {
	for _, t := range c.tools.Tools {
		var miss []string
		for _, need := range t.Needs {
			if strings.TrimSpace(c.lookup(need)) == "" {
				miss = append(miss, need)
			}
		}
		if len(miss) > 0 {
			c.missing[t.ID] = miss
		}
	}
}

// lookup 按 "secrets.x.y" / "paths.x" 的写法取值。
// secrets 的 key 由 flatten 展开时已带 "secrets." 前缀，直接查。
func (c *config) lookup(key string) string {
	if v, ok := c.secrets[key]; ok {
		return v
	}
	if v, ok := c.paths[strings.TrimPrefix(key, "paths.")]; ok && strings.HasPrefix(key, "paths.") {
		return v
	}
	return ""
}

func (c *config) resolve(s string) string {
	return placeholder.ReplaceAllStringFunc(s, func(m string) string {
		key := m[2 : len(m)-1]
		switch {
		case strings.HasPrefix(key, "secrets."):
			if v, ok := c.secrets[strings.TrimPrefix(key, "secrets.")]; ok {
				return v
			}
		case strings.HasPrefix(key, "paths."):
			if v, ok := c.paths[strings.TrimPrefix(key, "paths.")]; ok {
				return v
			}
		}
		return m
	})
}

func (c *config) toolDir(t Tool) string {
	if filepath.IsAbs(t.Dir) {
		return t.Dir
	}
	return filepath.Join(c.toolsDir, t.Dir)
}

func (c *config) toolURL(t Tool) string {
	if t.URL != "" {
		return c.resolve(t.URL)
	}
	if t.Port == 0 {
		return ""
	}
	return "http://127.0.0.1:" + strconv.Itoa(t.Port)
}
