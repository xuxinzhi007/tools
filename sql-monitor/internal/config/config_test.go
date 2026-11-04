package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func writeConfig(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReadRejectsMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, text := range []string{"null", "[]", "{", "{} {}", `{"unknown":1}`, `{"source":{"mode":"stdin","secret":"x"}}`} {
		writeConfig(t, path, text)
		if _, _, err := Read(path); err == nil {
			t.Errorf("accepted malformed config %q", text)
		}
	}
	writeConfig(t, path, `{}`+strings.Repeat(" ", 65536))
	if _, _, err := Read(path); err == nil {
		t.Error("accepted oversized config")
	}
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(fifo); err == nil {
		t.Error("accepted non-regular config")
	}
}

func TestLoadResolvesRelativePaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "monitor.json")
	writeConfig(t, path, `{"source":{"mode":"file","path":"logs/sql.log"}}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source.Path != filepath.Join(dir, "logs", "sql.log") {
		t.Fatalf("file path not resolved: %s", cfg.Source.Path)
	}

	writeConfig(t, path, `{"source":{"mode":"k8s","k8s":{"environments":[{"name":"dev","kubeconfig":"./kube.yml"}]}}}`)
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source.K8s.Environments[0].Kubeconfig != filepath.Join(dir, "kube.yml") {
		t.Fatalf("kubeconfig not resolved: %s", cfg.Source.K8s.Environments[0].Kubeconfig)
	}
}

func TestLoadSourceValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.json")
	for _, text := range []string{
		`{"source":{"mode":"network"}}`,
		`{"source":{"mode":"file"}}`,
		`{"source":{"mode":"file","path":"x","k8s":{}}}`,
		`{"source":{"mode":"stdin","path":"x"}}`,
		`{"source":{"mode":"stdin","k8s":{}}}`,
		`{"source":{"mode":"k8s"}}`,
		`{"source":{"mode":"k8s","path":"x","k8s":{}}}`,
	} {
		writeConfig(t, path, text)
		if _, err := Load(path); err == nil {
			t.Errorf("accepted invalid source %q", text)
		}
	}
}

func TestLoadFeishu(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.json")
	writeConfig(t, path, `{"feishu":{"enabled":false}}`)
	if _, err := LoadFeishu(path); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled feishu not rejected: %v", err)
	}
	writeConfig(t, path, `{}`)
	if _, err := LoadFeishu(path); err == nil {
		t.Fatal("missing feishu section accepted")
	}
	writeConfig(t, path, `{"feishu":{"enabled":true,"app_id":"a","app_secret":"s","project":"p","interval_seconds":300,"slow_ms":200,"recipients":[{"email":"a@example.com"}]}}`)
	cfg, err := LoadFeishu(path)
	if err != nil || !cfg.Enabled {
		t.Fatalf("valid feishu rejected: %v", err)
	}
}

func TestLoadFeishuThresholdDaily(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.json")
	base := `{"feishu":{"enabled":true,"app_id":"a","app_secret":"s","project":"p","interval_seconds":300,"slow_ms":200,"recipients":[{"email":"a@example.com"}],%s}}`

	writeConfig(t, path, fmt.Sprintf(base, `"threshold":{"enabled":true,"errors":20,"slow":10},"daily":{"enabled":true,"hour":23,"minute":30}`))
	cfg, err := LoadFeishu(path)
	if err != nil {
		t.Fatalf("valid threshold+daily rejected: %v", err)
	}
	if cfg.Threshold.WindowSeconds != 60 || cfg.Threshold.CooldownSeconds != 900 {
		t.Fatalf("threshold defaults not applied: %+v", cfg.Threshold)
	}
	if cfg.Daily.Hour != 23 || cfg.Daily.Minute != 30 {
		t.Fatalf("daily misparsed: %+v", cfg.Daily)
	}

	for _, text := range []string{
		fmt.Sprintf(base, `"threshold":{"enabled":true}`),                               // errors and slow both zero
		fmt.Sprintf(base, `"threshold":{"enabled":true,"errors":5,"window_seconds":2}`), // window out of range
		fmt.Sprintf(base, `"daily":{"enabled":true,"hour":24,"minute":0}`),              // hour out of range
	} {
		writeConfig(t, path, text)
		if _, err := LoadFeishu(path); err == nil {
			t.Errorf("accepted invalid threshold/daily config %q", text)
		}
	}
}

func TestLoadDevops(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.json")
	writeConfig(t, path, `{"devops":{"token":"abc"}}`)
	cfg, err := Load(path)
	if err != nil || cfg.Devops.Token != "abc" || cfg.Devops.Environment != "ns-cloud-test" || cfg.Devops.Service != "data-service" {
		t.Fatalf("devops: %+v %v", cfg, err)
	}
	writeConfig(t, path, `{"devops":{"token":"Bearer abc"}}`)
	cfg, err = Load(path)
	if err != nil || cfg.Devops.Token != "abc" {
		t.Fatalf("bearer prefix: %+v %v", cfg.Devops, err)
	}
	writeConfig(t, path, `{"devops":{"token":""}}`)
	if _, err := Load(path); err == nil {
		t.Fatal("accepted empty devops token")
	}
	writeConfig(t, path, `{"devops":{"token":"abc"}}`)
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("accepted world-readable devops token")
	}
}

func TestSaveDevopsKeepsOtherSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sql-monitor.local.json")
	writeConfig(t, path, `{"source":{"mode":"file","path":"/tmp/data-service-local.log"},"port":8767}`)
	if err := SaveDevops(path, DevopsConfig{Token: "Bearer saved-token", Environment: "dev"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("saved config is not private")
	}
	cfg, err := Load(path)
	if err != nil || cfg.Devops.Token != "saved-token" || cfg.Devops.Environment != "ns-cloud-dev" || cfg.Devops.Service != "data-service" {
		t.Fatalf("saved devops: %+v %v", cfg.Devops, err)
	}
	if cfg.Source == nil || cfg.Source.Path != "/tmp/data-service-local.log" || cfg.Port == nil || *cfg.Port != 8767 {
		t.Fatal("save rewrote unrelated config")
	}
	if err := SaveDevops(filepath.Join(dir, "devops.example.json"), DevopsConfig{Token: "abc"}); err == nil {
		t.Fatal("wrote an example config")
	}
}

func TestLoadFeishuRequiresPrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.json")
	writeConfig(t, path, `{"feishu":{"enabled":true,"app_id":"a","app_secret":"s"}}`)
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFeishu(path); err == nil {
		t.Fatal("accepted world-readable app_secret")
	}
}

func TestFindEnvironment(t *testing.T) {
	cfg := &K8sConfig{Environments: []K8sEnvironment{{Name: "dev"}, {Name: "test"}}}
	if env, err := FindEnvironment(cfg, "test"); err != nil || env.Name != "test" {
		t.Fatalf("lookup: %+v %v", env, err)
	}
	if _, err := FindEnvironment(cfg, "nope"); err == nil || !strings.Contains(err.Error(), "available: dev, test") {
		t.Fatalf("missing env error: %v", err)
	}
	if _, err := FindEnvironment(nil, "x"); err == nil {
		t.Fatal("nil config lookup succeeded")
	}
}

func TestValidateLabel(t *testing.T) {
	if err := ValidateLabel("lane", "dev2"); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"a,b", "a=b"} {
		if err := ValidateLabel("lane", v); err == nil {
			t.Fatalf("accepted label value %q", v)
		}
	}
}

func TestLoadAnalysis(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.json")

	writeConfig(t, path, `{"analysis":{"enabled":true,"command":["python3","/abs/analyze.py"]}}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("valid analysis rejected: %v", err)
	}
	if cfg.Analysis.TimeoutSeconds != 600 || cfg.Analysis.MaxHotspots != 5 {
		t.Fatalf("analysis defaults not applied: %+v", cfg.Analysis)
	}

	for _, text := range []string{
		`{"analysis":{"enabled":true}}`,                                      // no command
		`{"analysis":{"enabled":true,"command":["python3",""]}}`,             // empty arg
		`{"analysis":{"enabled":true,"command":["x"],"cwd":"relative/dir"}}`, // relative cwd
		`{"analysis":{"enabled":true,"command":["x"],"timeout_seconds":5}}`,  // timeout too small
		`{"analysis":{"enabled":true,"command":["x"],"max_hotspots":30}}`,    // hotspots too large
	} {
		writeConfig(t, path, text)
		if _, err := Load(path); err == nil {
			t.Errorf("accepted invalid analysis config %q", text)
		}
	}

	writeConfig(t, path, `{"analysis":{"enabled":false}}`)
	cfg, err = Load(path)
	if err != nil || cfg.Analysis == nil {
		t.Fatalf("disabled analysis should load: %v %+v", err, cfg)
	}
}
