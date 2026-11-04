package config

import (
	"testing"
)

func TestDefaultEnvironments(t *testing.T) {
	cfg := NewDefaultConfig()

	if cfg.DefaultTenantID != "888888" {
		t.Fatalf("expected DefaultTenantID to be 888888, got %s", cfg.DefaultTenantID)
	}

	var foundDev, foundTest, foundLocal bool
	for _, env := range cfg.Environments {
		switch env.ID {
		case "dev":
			foundDev = true
			if env.BaseURL != "https://dev-api.x2robot.com" {
				t.Errorf("expected dev baseURL to be https://dev-api.x2robot.com, got %s", env.BaseURL)
			}
			if env.RoutePrefix != "/core" {
				t.Errorf("expected dev routePrefix to be /core, got %s", env.RoutePrefix)
			}
		case "test":
			foundTest = true
			if env.BaseURL != "https://test-api.x2robot.com" {
				t.Errorf("expected test baseURL to be https://test-api.x2robot.com, got %s", env.BaseURL)
			}
			if env.RoutePrefix != "/core" {
				t.Errorf("expected test routePrefix to be /core, got %s", env.RoutePrefix)
			}
		case "local":
			foundLocal = true
			if env.RoutePrefix != "" {
				t.Errorf("expected local routePrefix to be empty, got %s", env.RoutePrefix)
			}
		}
	}

	if !foundDev || !foundTest || !foundLocal {
		t.Errorf("missing environments: dev=%v, test=%v, local=%v", foundDev, foundTest, foundLocal)
	}
}
