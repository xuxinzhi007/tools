package proxy

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RequestPayload represents the API test request sent from UI
type RequestPayload struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	TraceID string            `json:"trace_id"`
	Timeout int               `json:"timeout_seconds"` // in seconds, default 60
}

// ResponsePayload represents the result returned to the UI
type ResponsePayload struct {
	Status     int               `json:"status"`
	StatusText string            `json:"status_text"`
	DurationMs float64           `json:"duration_ms"`
	SizeBytes  int64             `json:"size_bytes"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	TraceID    string            `json:"trace_id"`
	Error      string            `json:"error,omitempty"`
}

// Proxy handles forwarding requests to target services
type Proxy struct {
	client *http.Client
}

// NewProxy creates a new proxy instance
func NewProxy() *Proxy {
	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			host := req.URL.Hostname()
			if host == "localhost" || host == "127.0.0.1" ||
				strings.HasSuffix(host, ".x2robot.com") ||
				strings.HasSuffix(host, ".x2robot.cn") ||
				strings.HasSuffix(host, ".local") ||
				strings.HasPrefix(host, "10.") ||
				strings.HasPrefix(host, "172.") ||
				strings.HasPrefix(host, "192.168.") {
				return nil, nil
			}
			return http.ProxyFromEnvironment(req)
		},
	}

	return &Proxy{
		client: &http.Client{
			Transport: transport,
			Timeout:   120 * time.Second,
		},
	}
}

// generateTraceID generates a 32-character hex trace ID compatible with OpenTelemetry
func generateTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Execute performs the proxied HTTP request
func (p *Proxy) Execute(reqPayload *RequestPayload) *ResponsePayload {
	traceID := reqPayload.TraceID
	if traceID == "" {
		traceID = generateTraceID()
	}

	method := strings.ToUpper(strings.TrimSpace(reqPayload.Method))
	if method == "" {
		method = "GET"
	}

	var reqBody io.Reader
	if reqPayload.Body != "" && method != "GET" && method != "HEAD" {
		reqBody = bytes.NewBufferString(reqPayload.Body)
	}

	httpReq, err := http.NewRequest(method, reqPayload.URL, reqBody)
	if err != nil {
		return &ResponsePayload{
			Status:     0,
			StatusText: "Invalid Request",
			TraceID:    traceID,
			Error:      fmt.Sprintf("Failed to construct request: %v", err),
		}
	}

	// Set headers
	hasContentType := false
	for k, v := range reqPayload.Headers {
		if strings.EqualFold(k, "content-type") {
			hasContentType = true
		}
		httpReq.Header.Set(k, v)
	}

	if !hasContentType && reqPayload.Body != "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Always propagate trace id so Kratos / OTel tracing will attach it
	if httpReq.Header.Get("traceparent") == "" {
		// Standard W3C Trace Context: 00-{trace_id}-{span_id}-01
		spanID := traceID[:16]
		httpReq.Header.Set("traceparent", fmt.Sprintf("00-%s-%s-01", traceID, spanID))
	}
	if httpReq.Header.Get("x-trace-id") == "" {
		httpReq.Header.Set("x-trace-id", traceID)
	}

	// Execute with timing
	start := time.Now()
	resp, err := p.client.Do(httpReq)
	duration := time.Since(start)
	durationMs := float64(duration.Microseconds()) / 1000.0

	if err != nil {
		return &ResponsePayload{
			Status:     0,
			StatusText: "Network Error",
			DurationMs: durationMs,
			TraceID:    traceID,
			Error:      fmt.Sprintf("Request failed: %v", err),
		}
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return &ResponsePayload{
			Status:     resp.StatusCode,
			StatusText: resp.Status,
			DurationMs: durationMs,
			TraceID:    traceID,
			Error:      fmt.Sprintf("Failed to read response body: %v", err),
		}
	}

	respHeaders := make(map[string]string)
	for k, v := range resp.Header {
		respHeaders[k] = strings.Join(v, ", ")
	}

	return &ResponsePayload{
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		DurationMs: durationMs,
		SizeBytes:  int64(len(respBytes)),
		Headers:    respHeaders,
		Body:       string(respBytes),
		TraceID:    traceID,
	}
}
