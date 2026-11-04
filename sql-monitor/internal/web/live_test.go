package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLiveHTTPPrivacyAndPages(t *testing.T) {
	m, h := httpFixture(t)
	m.Consume([]byte("{\"level\":\"WARN\",\"msg\":\"PRIVATE_LIVE_MESSAGE\"}\n"))
	for _, path := range []string{"/", "/overview", "/requests", "/live"} {
		r := request(h, "GET", path)
		if r.Code != 200 || !bytes.Equal(r.Body.Bytes(), LiveHTML) {
			t.Fatalf("missing page %s", path)
		}
		if strings.Contains(r.Body.String(), "PRIVATE_LIVE_MESSAGE") {
			t.Fatal("raw content embedded in HTML")
		}
	}
	for _, path := range []string{"/api/live", "/api/live?raw=0", "/api/live?raw=1&other=1", "/api/live?raw=1&raw=1", "/api/live?raw=%31", "/api/%6cive?raw=1"} {
		r := request(h, "GET", path)
		if r.Code == 200 || strings.Contains(r.Body.String(), "PRIVATE_LIVE_MESSAGE") {
			t.Fatalf("unguarded live API: %s", path)
		}
	}
	if r := request(h, "GET", "/api/state"); strings.Contains(r.Body.String(), "PRIVATE_LIVE_MESSAGE") {
		t.Fatal("existing SQL API leaked general logs")
	}
	r := request(h, "GET", "/api/live?raw=1")
	if r.Code != 200 || !strings.Contains(r.Body.String(), "PRIVATE_LIVE_MESSAGE") || r.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("explicit live API missing")
	}
	if r := request(h, "GET", "/sql"); r.Code != 200 || !bytes.Equal(r.Body.Bytes(), IndexHTML) {
		t.Fatal("SQL page regression")
	}
	req := httptest.NewRequest("GET", "/api/live?raw=1", nil)
	req.Host = "127.0.0.1:8767"
	req.Header.Set("Origin", "https://untrusted.example")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatal("untrusted origin allowed")
	}
	if r := request(h, "POST", "/api/live?raw=1"); r.Code != 405 {
		t.Fatal("live writes allowed")
	}
}

func TestFeedSwitchIsGlobal(t *testing.T) {
	_, h := httpFixture(t)
	h.remote.Logs().Consume([]byte("{\"msg\":\"SQL Query\",\"sql\":\"SELECT 'remote-only'\",\"elapsed\":\"10ms\",\"source\":\"svc.go:1\"}\n"))
	h.remote.Logs().Consume([]byte("{\"level\":\"INFO\",\"msg\":\"REMOTE_LIVE_ONLY\"}\n"))
	local := request(h, "GET", "/api/state").Body.String()
	if !strings.Contains(local, "internal/data/tag.go:549") || strings.Contains(local, "svc.go:1") {
		t.Fatalf("default SQL state should stay on the local monitor: %s", local)
	}
	req := httptest.NewRequest("POST", "/api/feed", strings.NewReader(`{"source":"remote"}`))
	req.Host = "127.0.0.1:8767"
	req.Header.Set("Origin", "http://127.0.0.1:8767")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"source":"remote"`) {
		t.Fatalf("feed switch: %d %s", w.Code, w.Body.String())
	}
	remote := request(h, "GET", "/api/state").Body.String()
	if strings.Contains(remote, "internal/data/tag.go:549") || !strings.Contains(remote, "svc.go:1") {
		t.Fatalf("SQL state did not follow the remote source: %s", remote)
	}
	live := request(h, "GET", "/api/live?raw=1").Body.String()
	if strings.Contains(live, "internal/data/tag.go:549") || !strings.Contains(live, "REMOTE_LIVE_ONLY") {
		t.Fatalf("live view did not follow the remote source: %s", live)
	}
	status := request(h, "GET", "/api/status").Body.String()
	if !strings.Contains(status, `"source_mode":"remote"`) {
		t.Fatal("status stayed local")
	}
	back := httptest.NewRequest("POST", "/api/feed", strings.NewReader(`{"source":"local"}`))
	back.Host = "127.0.0.1:8767"
	back.Header.Set("Origin", "http://127.0.0.1:8767")
	bw := httptest.NewRecorder()
	h.ServeHTTP(bw, back)
	again := request(h, "GET", "/api/state").Body.String()
	if !strings.Contains(again, "internal/data/tag.go:549") || strings.Contains(again, "svc.go:1") {
		t.Fatalf("switching back to local did not restore the file monitor: %s", again)
	}
}

func TestDevopsTokenSavedAndReused(t *testing.T) {
	_, h := httpFixture(t)
	path := filepath.Join(t.TempDir(), "sql-monitor.local.json")
	h.configPath = path
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"logs":[]}}`))
	}))
	defer srv.Close()
	h.remote.SetURL(srv.URL)
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/devops", strings.NewReader(body))
		req.Host = "127.0.0.1:8767"
		req.Header.Set("Origin", "http://127.0.0.1:8767")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if post(`{"environment":"test"}`).Code != 400 {
		t.Fatal("accepted a switch without a token")
	}
	saved := post(`{"token":"saved-token","environment":"test","service":"data-service"}`)
	if saved.Code != 200 || !strings.Contains(saved.Body.String(), `"saved":true`) || strings.Contains(saved.Body.String(), "saved-token") {
		t.Fatalf("save response: %d %s", saved.Code, saved.Body.String())
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("token file is not private")
	}
	file, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(file), "saved-token") {
		t.Fatal("token was not written")
	}
	got := request(h, "GET", "/api/devops")
	if got.Code != 200 || strings.Contains(got.Body.String(), "saved-token") || !strings.Contains(got.Body.String(), `"token_set":true`) || !strings.Contains(got.Body.String(), "ns-cloud-test") {
		t.Fatalf("settings leaked or missed the token flag: %s", got.Body.String())
	}
	switched := post(`{"environment":"prod"}`)
	if switched.Code != 200 || !strings.Contains(switched.Body.String(), "ns-cloud-prod") || strings.Contains(switched.Body.String(), "saved-token") {
		t.Fatalf("environment switch: %d %s", switched.Code, switched.Body.String())
	}
}
