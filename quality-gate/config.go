package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type Config struct {
	Server ServerConfig `json:"server"`
	Ngrok  NgrokConfig  `json:"ngrok"`
}

type ServerConfig struct {
	Port      string `json:"port"`
	Root      string `json:"root"`
	AuthToken string `json:"auth_token"`
}

type NgrokConfig struct {
	AutoStart    bool   `json:"auto_start"`
	APIURL       string `json:"api_url"`
	PollInterval string `json:"poll_interval"`
	URLFile      string `json:"url_file"`
}

var appConfig Config

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Port: "8080",
			Root: "./workspace",
		},
		Ngrok: NgrokConfig{
			AutoStart:    false,
			PollInterval: "3s",
			URLFile:      "tunnel.url",
		},
	}
}

func loadConfig() Config {
	configPath := flag.String("config", envOr("CONFIG_PATH", "config.json"), "path to config file")
	flag.Parse()

	cfg := defaultConfig()

	data, err := os.ReadFile(*configPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("config file %s not found, using defaults (env vars still apply)", *configPath)
		} else {
			log.Printf("read config %s: %v, using defaults", *configPath, err)
		}
	} else if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("parse config %s: %v", *configPath, err)
	}

	applyEnvOverrides(&cfg)
	return cfg
}

func applyEnvOverrides(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("PORT")); v != "" {
		cfg.Server.Port = v
	}
	if v := strings.TrimSpace(os.Getenv("FILE_MCP_ROOT")); v != "" {
		cfg.Server.Root = v
	}
	if v := strings.TrimSpace(os.Getenv("MCP_AUTH_TOKEN")); v != "" {
		cfg.Server.AuthToken = v
	}
	if v := strings.TrimSpace(os.Getenv("NGROK_API_URL")); v != "" {
		cfg.Ngrok.APIURL = v
	}
	if v := strings.TrimSpace(os.Getenv("NGROK_URL_FILE")); v != "" {
		cfg.Ngrok.URLFile = v
	}
	if v := strings.TrimSpace(os.Getenv("NGROK_POLL_INTERVAL")); v != "" {
		cfg.Ngrok.PollInterval = v
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_START_NGROK")); v != "" {
		cfg.Ngrok.AutoStart = parseBool(v)
	}
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (c Config) pollInterval() time.Duration {
	d, err := time.ParseDuration(c.Ngrok.PollInterval)
	if err != nil || d <= 0 {
		return 3 * time.Second
	}
	return d
}

func (c Config) serverPort() string {
	return strings.TrimPrefix(strings.TrimSpace(c.Server.Port), ":")
}

func (c Config) configSummary() string {
	return fmt.Sprintf("config: port=%s root=%s ngrok.auto_start=%t",
		c.serverPort(), c.Server.Root, c.Ngrok.AutoStart)
}
