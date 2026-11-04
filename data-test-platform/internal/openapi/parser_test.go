package openapi

import (
	"os"
	"testing"
)

// 这个测试依赖真实的 data-service 仓库产物，不在本仓库内。
// 找不到就跳过，避免在独立 checkout 或 CI 里必然失败。
func TestParseDataServiceOpenAPI(t *testing.T) {
	const path = "../../../data-service/openapi.yaml"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("skipped: %s not found (%v)", path, err)
	}

	p := NewParser(path)
	spec, err := p.ParseFromFile("")
	if err != nil {
		t.Fatalf("ParseFromFile error: %v", err)
	}

	if spec.TotalCount == 0 {
		t.Fatalf("expected endpoints > 0, got 0")
	}

	t.Logf("Total endpoints parsed: %d across %d tags", spec.TotalCount, len(spec.Tags))
	for _, g := range spec.Groups {
		t.Logf("  Group %-25s: %2d endpoints", g.Name, g.Count)
	}

	// Verify sample mock body
	for _, ep := range spec.Endpoints {
		if ep.MockBodyJSON != "" {
			t.Logf("Sample mock body for %s %s:\n%s", ep.Method, ep.Path, ep.MockBodyJSON)
			break
		}
	}
}
