package logger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHubLocalGate(t *testing.T) {
	parser := NewLogParser(200 * time.Millisecond)
	hub := NewHub(parser)

	// 未设置门控时本地行正常进入
	line := `{"level":"INFO","ts":"2026-10-15T14:32:01.234+08:00","source":"internal/data/job.go:134","msg":"hello"}`
	if entry := hub.IngestRawLine(line); entry == nil {
		t.Fatal("expected entry to be ingested when no gate is set")
	}

	// 设置门控:远端来源激活 (gate=false) 时本地行被丢弃
	hub.SetLocalGate(func() bool { return false })
	if entry := hub.IngestRawLine(line); entry != nil {
		t.Fatalf("expected local line to be dropped when gate closed, got %+v", entry)
	}

	// 门控恢复后可继续进入
	hub.SetLocalGate(func() bool { return true })
	if entry := hub.IngestRawLine(line); entry == nil {
		t.Fatal("expected entry to be ingested when gate open")
	}
}

func TestHubLocalGateBlocksBroadcast(t *testing.T) {
	parser := NewLogParser(200 * time.Millisecond)
	hub := NewHub(parser)

	// WS 服务端
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		hub.RegisterClient(conn)
	}))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close()

	// 等待注册完成
	time.Sleep(100 * time.Millisecond)

	hub.SetLocalGate(func() bool { return false })
	hub.IngestRawLine(`{"level":"INFO","msg":"should be dropped"}`)

	// 门控开启的行
	hub.SetLocalGate(func() bool { return true })
	hub.IngestRawLine(`{"level":"INFO","msg":"should arrive"}`)

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var entry LogEntry
	if err := json.Unmarshal(msg, &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.Message != "should arrive" {
		t.Fatalf("expected 'should arrive', got %q", entry.Message)
	}
}
