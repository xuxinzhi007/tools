package logger

import (
	"strings"
	"testing"
)

func TestK8sTargetSelector(t *testing.T) {
	cases := []struct {
		name   string
		target K8sTarget
		want   string
	}{
		{"无 lane 看全部 pod", K8sTarget{App: "data-service"}, "app=data-service"},
		{"指定 lane", K8sTarget{App: "data-service", Lane: "dev2"}, "app=data-service,x2robot.io/lane=dev2"},
		{"默认 app", K8sTarget{}, "app=data-service"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.target.withDefaults().Selector()
			if got != c.want {
				t.Fatalf("Selector() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestK8sTargetArgs(t *testing.T) {
	target := K8sTarget{
		Kubectl:    "kubectl",
		Kubeconfig: "/tmp/kc.yml",
		Context:    "dev-user-context",
		Namespace:  "ns-cloud-dev",
		App:        "data-service",
		Lane:       "dev2",
		Container:  "app",
	}.withDefaults()

	got := strings.Join(target.args(200, true), " ")
	want := "--kubeconfig /tmp/kc.yml --context dev-user-context -n ns-cloud-dev " +
		"logs -f --tail=200 -l app=data-service,x2robot.io/lane=dev2 --container=app"
	if got != want {
		t.Fatalf("args = %q\nwant %q", got, want)
	}
}

func TestK8sTargetArgsDefaults(t *testing.T) {
	target := K8sTarget{App: "data-service"}.withDefaults()
	got := strings.Join(target.args(0, true), " ")
	want := "logs -f --tail=0 -l app=data-service"
	if got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}
}

func TestSplitK8sPrefix(t *testing.T) {
	cases := []struct {
		line string
		pod  string
		body string
	}{
		{
			line: `data-service-dev2-78d76496db-mscw6 {"level":"INFO","msg":"hi"}`,
			pod:  "data-service-dev2-78d76496db-mscw6",
			body: `{"level":"INFO","msg":"hi"}`,
		},
		{
			// 单 pod 时 kubectl 不加前缀，JSON 原样
			line: `{"level":"INFO","msg":"hi"}`,
			pod:  "",
			body: `{"level":"INFO","msg":"hi"}`,
		},
		{
			// 非 JSON 的紧凑格式也不该被误剥
			line: `INFO ts=123 msg=hello`,
			pod:  "",
			body: `INFO ts=123 msg=hello`,
		},
	}
	for _, c := range cases {
		pod, body := splitK8sPrefix(c.line)
		if pod != c.pod || body != c.body {
			t.Fatalf("splitK8sPrefix(%q) = (%q, %q), want (%q, %q)",
				c.line, pod, body, c.pod, c.body)
		}
	}
}

func TestBoundedErr(t *testing.T) {
	b := &boundedErr{limit: 8}
	n, err := b.Write([]byte("0123456789abcdef"))
	if err != nil || n != 16 {
		t.Fatalf("Write returned (%d, %v), want (16, nil)", n, err)
	}
	if got := b.String(); got != "01234567" {
		t.Fatalf("String() = %q, want %q", got, "01234567")
	}
}

func TestK8sTailerStatusIdle(t *testing.T) {
	k := NewK8sTailer(NewHub(NewLogParser(200)))
	running, state, _, _ := k.Status()
	if running {
		t.Fatal("新实例不应在运行")
	}
	if state != "idle" {
		t.Fatalf("state = %q, want idle", state)
	}
}
