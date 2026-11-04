package config

import (
	"os"
	"path/filepath"
	"testing"
)

// 默认配置必须是中性的：不带任何具体项目的域名、租户或账号。
// 项目相关信息一律由 --project 指向的 profile 提供。
func TestDefaultConfigIsProjectNeutral(t *testing.T) {
	cfg := NewDefaultConfig()

	if cfg.DefaultTenantID != "" {
		t.Errorf("DefaultTenantID 应为空，实际 %q", cfg.DefaultTenantID)
	}
	if cfg.EnableXrAuth {
		t.Error("EnableXrAuth 应默认关闭")
	}
	if len(cfg.ProfileHeaders) != 0 {
		t.Errorf("默认不应无配置就注入请求头，实际 %d 条", len(cfg.ProfileHeaders))
	}
	for _, env := range cfg.Environments {
		switch env.ID {
		case "dev", "test", "prod":
			t.Errorf("默认不应内置 %s 环境，应由 profile 声明", env.ID)
		case "local", "custom":
			// 这两个是通用兜底，允许存在
		default:
			t.Errorf("未预期的默认环境 %q", env.ID)
		}
	}
}

func TestApplyProfile(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.ApplyProfile(&ProjectProfile{
		Name:  "demo-svc",
		Title: "Demo 平台",
		Vars:  map[string]string{"tenant": "42", "user": "a@b.c"},
		Environments: []Environment{
			{ID: "dev", Name: "Dev", BaseURL: "https://dev.demo.io", RoutePrefix: "/api"},
		},
		Headers: []HeaderRule{
			{Key: "tenant-id", Value: "${tenant}", Desc: "租户"},
			{Key: "x-sub", Value: "${user}", Desc: "用户"},
		},
		RemoteLog: &RemoteLogConfig{
			WorkflowURL: "https://obs.demo.io/query",
			Service:     "demo-svc",
		},
	})

	if cfg.ProjectName != "demo-svc" {
		t.Errorf("ProjectName = %q", cfg.ProjectName)
	}
	if len(cfg.Environments) != 1 || cfg.Environments[0].BaseURL != "https://dev.demo.io" {
		t.Errorf("Environments 未按 profile 覆盖: %+v", cfg.Environments)
	}
	if cfg.RemoteLog.WorkflowURL != "https://obs.demo.io/query" {
		t.Errorf("RemoteLog 未按 profile 覆盖")
	}

	p := &ProjectProfile{Vars: cfg.ProfileVars}
	if got := p.ExpandVars("${tenant}"); got != "42" {
		t.Errorf("ExpandVars(${tenant}) = %q, want 42", got)
	}
	if got := p.ExpandVars("${user}"); got != "a@b.c" {
		t.Errorf("ExpandVars(${user}) = %q, want a@b.c", got)
	}
	// 未定义的变量保留原文，方便前端提示配置缺失
	if got := p.ExpandVars("${missing}"); got != "${missing}" {
		t.Errorf("未定义变量应原样保留，实际 %q", got)
	}
	// 没有占位符时不改动
	if got := p.ExpandVars("plain-value"); got != "plain-value" {
		t.Errorf("普通值被改写: %q", got)
	}
}

func TestApplyNilProfileIsNoop(t *testing.T) {
	cfg := NewDefaultConfig()
	before := cfg.ProjectName
	cfg.ApplyProfile(nil)
	if cfg.ProjectName != before {
		t.Error("nil profile 不应改动配置")
	}
}

func TestLoadProfile(t *testing.T) {
	dir := t.TempDir()

	if p, err := LoadProfile(""); err != nil || p != nil {
		t.Errorf("空路径应返回 (nil, nil)，实际 (%v, %v)", p, err)
	}
	if p, err := LoadProfile(filepath.Join(dir, "nope.json")); err != nil || p != nil {
		t.Errorf("文件不存在应返回 (nil, nil)，实际 (%v, %v)", p, err)
	}

	path := filepath.Join(dir, "p.json")
	if err := os.WriteFile(path, []byte(`{"name":"x","vars":{"a":"1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if p.Name != "x" || p.Vars["a"] != "1" {
		t.Errorf("解析结果不符: %+v", p)
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfile(bad); err == nil {
		t.Error("非法 JSON 应报错")
	}
}
