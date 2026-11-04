package openapi

import (
	"testing"
)

func TestParseDataServiceOpenAPI(t *testing.T) {
	p := NewParser("../../../data-service/openapi.yaml")
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
