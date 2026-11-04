package directnet

import "testing"

func TestIsDirectBuiltin(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"10.1.2.3", true},
		{"192.168.1.5", true},
		{"172.16.0.9", true},  // 172.16/12 段内
		{"172.32.0.9", false}, // 段外，不应误判
		{"printer.local", true},
		{"example.com", false},
		{"169.254.1.1", true}, // 链路本地
		{"", false},
	}
	for _, c := range cases {
		if got := IsDirect(c.host); got != c.want {
			t.Errorf("IsDirect(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestExtraSuffixes(t *testing.T) {
	// 恢复全局状态，避免测试互相影响
	defer SetExtraSuffixes(nil)

	SetExtraSuffixes([]string{"example.com", ".internal.io", "  "})

	cases := []struct {
		host string
		want bool
	}{
		{"api.example.com", true},
		{"a.b.example.com", true},
		{"svc.internal.io", true},
		{"example.org", false},
		{"notexample.com.evil.io", false},
	}
	for _, c := range cases {
		if got := IsDirect(c.host); got != c.want {
			t.Errorf("IsDirect(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestSuffixesCleared(t *testing.T) {
	SetExtraSuffixes([]string{"corp.example"})
	if !IsDirect("x.corp.example") {
		t.Fatal("后缀应生效")
	}
	SetExtraSuffixes(nil)
	if IsDirect("x.corp.example") {
		t.Fatal("清空后不应仍命中")
	}
}
