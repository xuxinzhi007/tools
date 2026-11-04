package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type tunnelState struct {
	mu         sync.RWMutex
	publicURL  string
	mcpURL     string
	apiURL     string
	lastChange time.Time
}

var ngrok tunnelState

func startNgrokWatcher() {
	interval := appConfig.pollInterval()
	urlFile := appConfig.Ngrok.URLFile

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			apiURL, publicURL := discoverNgrokTunnel()
			if publicURL != "" {
				updateTunnelURL(apiURL, publicURL, urlFile)
			}
			<-ticker.C
		}
	}()
}

func maybeStartNgrok(port string) {
	if !appConfig.Ngrok.AutoStart {
		return
	}

	if _, publicURL := discoverNgrokTunnel(); publicURL != "" {
		log.Printf("ngrok already running (%s), skip auto-start", publicURL)
		return
	}

	log.Printf("ngrok.auto_start=true in config.json, launching: ngrok http %s", port)
	cmd := exec.Command("ngrok", "http", port)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("auto-start ngrok failed: %v (ensure ngrok is in PATH)", err)
		return
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("ngrok process exited: %v", err)
		}
	}()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, publicURL := discoverNgrokTunnel(); publicURL != "" {
			log.Printf("ngrok ready: %s", publicURL)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("ngrok started but tunnel not detected yet; watcher will retry")
}

func discoverNgrokTunnel() (string, string) {
	if custom := strings.TrimSpace(appConfig.Ngrok.APIURL); custom != "" {
		if u, ok := fetchNgrokPublicURL(custom); ok {
			return custom, u
		}
	}

	for p := 4040; p <= 4050; p++ {
		apiURL := fmt.Sprintf("http://127.0.0.1:%d/api/tunnels", p)
		if u, ok := fetchNgrokPublicURL(apiURL); ok {
			return apiURL, u
		}
	}
	return "", ""
}

func fetchNgrokPublicURL(apiURL string) (string, bool) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false
	}

	var payload struct {
		Tunnels []struct {
			PublicURL string `json:"public_url"`
			Proto     string `json:"proto"`
			Config    struct {
				Addr string `json:"addr"`
			} `json:"config"`
		} `json:"tunnels"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", false
	}
	if len(payload.Tunnels) == 0 {
		return "", false
	}

	port := appConfig.serverPort()
	for _, t := range payload.Tunnels {
		if t.Proto != "https" {
			continue
		}
		if strings.Contains(t.Config.Addr, ":"+port) {
			return strings.TrimRight(t.PublicURL, "/"), true
		}
	}
	for _, t := range payload.Tunnels {
		if t.Proto == "https" {
			return strings.TrimRight(t.PublicURL, "/"), true
		}
	}
	return "", false
}

func updateTunnelURL(apiURL, publicURL, urlFile string) {
	mcpURL := publicURL + "/mcp"

	ngrok.mu.RLock()
	changed := ngrok.publicURL != publicURL || ngrok.apiURL != apiURL
	ngrok.mu.RUnlock()
	if !changed {
		return
	}

	ngrok.mu.Lock()
	ngrok.publicURL = publicURL
	ngrok.mcpURL = mcpURL
	ngrok.apiURL = apiURL
	ngrok.lastChange = time.Now()
	ngrok.mu.Unlock()

	if err := os.WriteFile(urlFile, []byte(mcpURL+"\n"), 0o644); err != nil {
		log.Printf("write %s: %v", urlFile, err)
	}

	log.Printf("ngrok detected via %s", apiURL)
	log.Printf("Grok connector URL: %s", mcpURL)
}

func currentTunnelInfo() ginH {
	ngrok.mu.RLock()
	defer ngrok.mu.RUnlock()

	sseURL := ""
	if ngrok.publicURL != "" {
		sseURL = ngrok.publicURL + "/sse"
	}

	return ginH{
		"public_url":  ngrok.publicURL,
		"mcp_url":     ngrok.mcpURL,
		"sse_url":     sseURL,
		"api_url":     ngrok.apiURL,
		"connected":   ngrok.publicURL != "",
		"last_change": ngrok.lastChange.Format(time.RFC3339),
		"grok_hint":   grokConnectorHint(ngrok.mcpURL),
	}
}

func grokConnectorHint(mcpURL string) string {
	if mcpURL == "" {
		return "Set ngrok.auto_start in config.json, or run: ngrok http 8080"
	}
	return fmt.Sprintf("grok.com/connectors → Custom → %s", mcpURL)
}

type ginH map[string]any
