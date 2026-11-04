package openapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MockGenerator generates realistic JSON mock payloads from OpenAPI schemas
type MockGenerator struct {
	schemas map[string]*Schema
}

// NewMockGenerator creates a new generator
func NewMockGenerator(schemas map[string]*Schema) *MockGenerator {
	return &MockGenerator{schemas: schemas}
}

// GenerateMockJSON generates formatted JSON for a schema
func (g *MockGenerator) GenerateMockJSON(schema *Schema) string {
	if schema == nil {
		return "{}"
	}
	mockData := g.generateValue(schema, "", 0)
	bytes, err := json.MarshalIndent(mockData, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

// resolveRef resolves $ref string like "#/components/schemas/xyz"
func (g *MockGenerator) resolveRef(ref string) *Schema {
	if ref == "" {
		return nil
	}
	parts := strings.Split(ref, "/")
	name := parts[len(parts)-1]
	if schema, ok := g.schemas[name]; ok {
		return schema
	}
	return nil
}

func (g *MockGenerator) generateValue(schema *Schema, fieldName string, depth int) interface{} {
	if schema == nil || depth > 5 {
		return nil
	}

	// If $ref is present, resolve target schema
	if schema.Ref != "" {
		resolved := g.resolveRef(schema.Ref)
		if resolved != nil {
			return g.generateValue(resolved, fieldName, depth+1)
		}
		return map[string]interface{}{}
	}

	// Check example or default
	if schema.Example != nil {
		return schema.Example
	}

	// Check enums
	if len(schema.Enum) > 0 {
		return schema.Enum[0]
	}

	lowerName := strings.ToLower(fieldName)

	// In OpenAPI, if type is omitted but properties are present, treat as object
	sType := schema.Type
	if sType == "" && len(schema.Properties) > 0 {
		sType = "object"
	}

	switch sType {
	case "object":
		result := make(map[string]interface{})
		for propName, propSchema := range schema.Properties {
			result[propName] = g.generateValue(propSchema, propName, depth+1)
		}
		return result

	case "array":
		if schema.Items != nil {
			itemVal := g.generateValue(schema.Items, fieldName+"_item", depth+1)
			if itemVal != nil {
				return []interface{}{itemVal}
			}
		}
		return []interface{}{}

	case "integer":
		if schema.Default != nil {
			return schema.Default
		}
		if strings.Contains(lowerName, "tenant") {
			return 1
		}
		if strings.Contains(lowerName, "user_id") || strings.Contains(lowerName, "operator_id") {
			return 1001
		}
		if strings.Contains(lowerName, "page_size") || strings.Contains(lowerName, "limit") {
			return 20
		}
		if strings.Contains(lowerName, "page") {
			return 1
		}
		if strings.Contains(lowerName, "status") {
			return 1
		}
		return 0

	case "number":
		if schema.Default != nil {
			return schema.Default
		}
		return 0.0

	case "boolean":
		if schema.Default != nil {
			return schema.Default
		}
		return true

	case "string":
		if schema.Default != nil {
			return schema.Default
		}
		if schema.Format == "date-time" || strings.Contains(lowerName, "time") || strings.Contains(lowerName, "date") {
			return time.Now().Format(time.RFC3339)
		}
		if strings.HasSuffix(lowerName, "_id") || strings.Contains(lowerName, "id") {
			if strings.Contains(lowerName, "job") {
				return "job_20260924_001"
			}
			if strings.Contains(lowerName, "task") {
				return "task_20260924_001"
			}
			if strings.Contains(lowerName, "episode") {
				return "ep_20260924_001"
			}
			if strings.Contains(lowerName, "flow") {
				return "flow_20260924_001"
			}
			if strings.Contains(lowerName, "node") {
				return "node_20260924_001"
			}
			return fmt.Sprintf("%s_demo", fieldName)
		}
		if strings.Contains(lowerName, "name") {
			return fmt.Sprintf("sample_%s", fieldName)
		}
		if strings.Contains(lowerName, "remark") || strings.Contains(lowerName, "comment") || strings.Contains(lowerName, "desc") {
			return "测试备注说明"
		}
		return "test_value"

	default:
		// If has properties
		if len(schema.Properties) > 0 {
			result := make(map[string]interface{})
			for propName, propSchema := range schema.Properties {
				result[propName] = g.generateValue(propSchema, propName, depth+1)
			}
			return result
		}
		return "string"
	}
}
