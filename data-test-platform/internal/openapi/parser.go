package openapi

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var pathParamRegex = regexp.MustCompile(`\{([a-zA-Z0-9_-]+)\}`)

// Parser handles loading and processing OpenAPI 3.0 specs
type Parser struct {
	filePath string
}

// NewParser creates a new OpenAPI parser
func NewParser(filePath string) *Parser {
	return &Parser{filePath: filePath}
}

// ParseFromFile loads and parses the OpenAPI file
func (p *Parser) ParseFromFile(filePath string) (*ParsedSpec, error) {
	if filePath == "" {
		filePath = p.filePath
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read openapi file '%s': %w", filePath, err)
	}

	fileInfo, _ := os.Stat(filePath)
	lastMod := time.Now().Format("2006-01-02 15:04:05")
	if fileInfo != nil {
		lastMod = fileInfo.ModTime().Format("2006-01-02 15:04:05")
	}

	return p.ParseBytes(data, lastMod)
}

// ParseBytes parses raw YAML or JSON bytes
func (p *Parser) ParseBytes(data []byte, lastMod string) (*ParsedSpec, error) {
	var raw RawOpenAPI
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal openapi yaml: %w", err)
	}

	spec := &ParsedSpec{
		Version:      raw.OpenAPI,
		Title:        "Data Service API",
		Schemas:      raw.Components.Schemas,
		EndpointMap:  make(map[string]*Operation),
		LastModified: lastMod,
	}

	if raw.Info != nil {
		if title, ok := raw.Info["title"].(string); ok && title != "" {
			spec.Title = title
		}
		if ver, ok := raw.Info["version"].(string); ok && ver != "" {
			spec.Version = ver
		}
	}

	mockGen := NewMockGenerator(raw.Components.Schemas)
	tagMap := make(map[string][]*Operation)
	var allEndpoints []*Operation

	validMethods := map[string]bool{
		"get":     true,
		"post":    true,
		"put":     true,
		"delete":  true,
		"patch":   true,
		"head":    true,
		"options": true,
	}

	// Iterate through paths
	for path, methods := range raw.Paths {
		for method, op := range methods {
			methodLower := strings.ToLower(method)
			if !validMethods[methodLower] {
				continue
			}

			// Copy operation to avoid pointer issue in loop
			operation := op
			operation.Method = strings.ToUpper(methodLower)
			operation.Path = path

			// Extract tag
			tag := "Default"
			if len(operation.Tags) > 0 && operation.Tags[0] != "" {
				tag = operation.Tags[0]
			} else {
				// Derive from path like /api/job/v1/... -> JobService
				segments := strings.Split(strings.Trim(path, "/"), "/")
				if len(segments) >= 2 && segments[0] == "api" {
					tag = strings.Title(segments[1]) + "Service"
				}
			}
			operation.Tag = tag

			// Extract Path Params from URL template {id}
			matches := pathParamRegex.FindAllStringSubmatch(path, -1)
			pathParams := make([]string, 0, len(matches))
			for _, m := range matches {
				if len(m) > 1 {
					pathParams = append(pathParams, m[1])
				}
			}
			operation.PathParams = pathParams

			// Separate query parameters
			queryParams := make([]Parameter, 0)
			for _, param := range operation.Parameters {
				if param.In == "query" {
					queryParams = append(queryParams, param)
				}
			}
			operation.QueryParams = queryParams

			// Generate Mock Request Body if present
			if operation.RequestBody != nil {
				if media, ok := operation.RequestBody.Content["application/json"]; ok && media.Schema != nil {
					operation.MockBodyJSON = mockGen.GenerateMockJSON(media.Schema)
				}
			}

			// Save to collections
			opKey := fmt.Sprintf("%s %s", operation.Method, operation.Path)
			spec.EndpointMap[opKey] = &operation
			allEndpoints = append(allEndpoints, &operation)
			tagMap[tag] = append(tagMap[tag], &operation)
		}
	}

	// Sort endpoints by Path then Method
	sort.Slice(allEndpoints, func(i, j int) bool {
		if allEndpoints[i].Path == allEndpoints[j].Path {
			return allEndpoints[i].Method < allEndpoints[j].Method
		}
		return allEndpoints[i].Path < allEndpoints[j].Path
	})
	spec.Endpoints = allEndpoints
	spec.TotalCount = len(allEndpoints)

	// Build Tag Groups sorted by Name
	var tags []string
	for t := range tagMap {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	spec.Tags = tags

	var groups []TagGroup
	for _, t := range tags {
		eps := tagMap[t]
		sort.Slice(eps, func(i, j int) bool {
			if eps[i].Path == eps[j].Path {
				return eps[i].Method < eps[j].Method
			}
			return eps[i].Path < eps[j].Path
		})
		groups = append(groups, TagGroup{
			Name:        t,
			Count:       len(eps),
			Endpoints:   eps,
			Description: fmt.Sprintf("%s 接口集合", t),
		})
	}
	spec.Groups = groups

	return spec, nil
}
