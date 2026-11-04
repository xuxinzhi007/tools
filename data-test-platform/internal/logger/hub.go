package logger

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const maxLogBufferSize = 10000

// Hub manages active WebSocket clients, the log ring buffer, and stats
type Hub struct {
	mu            sync.RWMutex
	clients       map[*websocket.Conn]bool
	buffer        []*LogEntry
	broadcastChan chan *LogEntry
	register      chan *websocket.Conn
	unregister    chan *websocket.Conn
	stats         *StatsCollector
	parser        *LogParser
	aggregates    *AggregateStore
	localGate     func() bool // when set and returning false, local-origin raw lines are dropped (e.g. remote log source active)
}

// NewHub creates a new Hub instance
func NewHub(parser *LogParser) *Hub {
	h := &Hub{
		clients:       make(map[*websocket.Conn]bool),
		buffer:        make([]*LogEntry, 0, maxLogBufferSize),
		broadcastChan: make(chan *LogEntry, 1024),
		register:      make(chan *websocket.Conn, 32),
		unregister:    make(chan *websocket.Conn, 32),
		stats:         NewStatsCollector(),
		parser:        parser,
		aggregates:    NewAggregateStore(200),
	}
	go h.run()
	return h
}

// Parser returns the associated parser
func (h *Hub) Parser() *LogParser {
	return h.parser
}

// Stats returns the stats collector
func (h *Hub) Stats() *StatsCollector {
	return h.stats
}

// Aggregates returns the cumulative SQL fingerprint aggregate store
func (h *Hub) Aggregates() *AggregateStore {
	return h.aggregates
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
			h.mu.Unlock()

		case entry := <-h.broadcastChan:
			// 未显式标注来源的入口 (本地文件 tail / 进程 stdout / 模拟日志 / 手动 ingest)
			// 统一标记为 local,与远端云日志 (dev/test/prod) 区分
			if entry != nil && entry.LogSource == "" {
				entry.LogSource = "local"
			}

			// Add to ring buffer
			h.mu.Lock()
			if len(h.buffer) >= maxLogBufferSize {
				h.buffer = h.buffer[1:]
			}
			h.buffer = append(h.buffer, entry)
			h.mu.Unlock()

			// Record stats and cumulative fingerprint aggregates
			h.stats.Record(entry)
			h.aggregates.Record(entry)

			// Broadcast to WS clients
			msgBytes, err := json.Marshal(entry)
			if err != nil {
				continue
			}

			h.mu.RLock()
			for client := range h.clients {
				client.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := client.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
					go func(c *websocket.Conn) {
						h.unregister <- c
					}(client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// SetLocalGate installs a gate for local-origin ingestion (file tail, process
// runner, manual ingest API). When the gate returns false, raw lines parsed
// from local sources are dropped instead of entering the buffer, stats and
// WebSocket broadcast. Remote entries via IngestEntry are not affected.
func (h *Hub) SetLocalGate(gate func() bool) {
	h.mu.Lock()
	h.localGate = gate
	h.mu.Unlock()
}

// RegisterClient adds a client to active connections
func (h *Hub) RegisterClient(conn *websocket.Conn) {
	h.register <- conn
}

// IngestRawLine parses raw string and broadcasts it
// IngestRawLine 解析一行原始日志并入 Hub，受 localGate 控制（本地文件源用）。
func (h *Hub) IngestRawLine(line string) *LogEntry {
	h.mu.RLock()
	gate := h.localGate
	h.mu.RUnlock()
	if gate != nil && !gate() {
		return nil
	}
	return h.ingest(line)
}

// IngestRawLineFromSource 解析一行原始日志并打上来源标记后入 Hub，绕过 localGate。
// K8s 这类显式选中的日志源用它 —— 必须先解析、标完字段再广播，
// 否则 Hub 已经把 entry 推给 WebSocket 客户端，后续赋值就是数据竞争。
func (h *Hub) IngestRawLineFromSource(line, source, podName string) *LogEntry {
	entry := h.parser.ParseLine(line)
	if entry == nil {
		return nil
	}
	if source != "" {
		entry.LogSource = source
	}
	if podName != "" {
		entry.PodName = podName
	}
	h.broadcastChan <- entry
	return entry
}

func (h *Hub) ingest(line string) *LogEntry {
	entry := h.parser.ParseLine(line)
	if entry != nil {
		h.broadcastChan <- entry
	}
	return entry
}

// IngestEntry broadcasts an already parsed entry
func (h *Hub) IngestEntry(entry *LogEntry) {
	if entry != nil {
		h.broadcastChan <- entry
	}
}

// GetRecentLogs retrieves recent logs with optional filtering
func (h *Hub) GetRecentLogs(limit int, level, search string, sqlOnly, slowOnly bool) []*LogEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if limit <= 0 || limit > len(h.buffer) {
		limit = len(h.buffer)
	}

	searchLower := strings.ToLower(search)
	levelUpper := strings.ToUpper(level)

	result := make([]*LogEntry, 0, limit)
	// Iterate from newest to oldest or chronological
	startIdx := len(h.buffer) - limit
	if startIdx < 0 {
		startIdx = 0
	}

	for i := startIdx; i < len(h.buffer); i++ {
		e := h.buffer[i]

		if levelUpper != "" && levelUpper != "ALL" && e.Level != levelUpper {
			continue
		}
		if sqlOnly && !e.IsSQL {
			continue
		}
		if slowOnly && !e.IsSlowSQL {
			continue
		}
		if searchLower != "" {
			match := strings.Contains(strings.ToLower(e.Message), searchLower) ||
				strings.Contains(strings.ToLower(e.SQL), searchLower) ||
				strings.Contains(strings.ToLower(e.Source), searchLower) ||
				strings.Contains(strings.ToLower(e.TraceID), searchLower) ||
				strings.Contains(strings.ToLower(e.Raw), searchLower)
			if !match {
				continue
			}
		}

		result = append(result, e)
	}

	return result
}

// Clear removes all buffered logs and resets stats
func (h *Hub) Clear() {
	h.mu.Lock()
	h.buffer = make([]*LogEntry, 0, maxLogBufferSize)
	h.mu.Unlock()

	h.stats.Clear()
	h.aggregates.Clear()
}
