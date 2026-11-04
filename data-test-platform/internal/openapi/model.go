package openapi

// Parameter represents an OpenAPI parameter (query, path, header)
type Parameter struct {
	Name        string  `json:"name" yaml:"name"`
	In          string  `json:"in" yaml:"in"` // query, path, header
	Description string  `json:"description" yaml:"description"`
	Required    bool    `json:"required" yaml:"required"`
	Schema      *Schema `json:"schema" yaml:"schema"`
}

// MediaType represents requestBody/response media type
type MediaType struct {
	Schema *Schema `json:"schema" yaml:"schema"`
}

// RequestBody represents an OpenAPI request body
type RequestBody struct {
	Description string               `json:"description" yaml:"description"`
	Content     map[string]MediaType `json:"content" yaml:"content"`
	Required    bool                 `json:"required" yaml:"required"`
}

// Response represents an OpenAPI response
type Response struct {
	Description string               `json:"description" yaml:"description"`
	Content     map[string]MediaType `json:"content" yaml:"content"`
}

// Operation represents an API operation (endpoint + method)
type Operation struct {
	ID          string              `json:"operationId" yaml:"operationId"`
	Tags        []string            `json:"tags" yaml:"tags"`
	Summary     string              `json:"summary" yaml:"summary"`
	Description string              `json:"description" yaml:"description"`
	Parameters  []Parameter         `json:"parameters" yaml:"parameters"`
	RequestBody *RequestBody        `json:"requestBody" yaml:"requestBody"`
	Responses   map[string]Response `json:"responses" yaml:"responses"`

	// Derived fields for frontend convenience
	Method       string                 `json:"method"`
	Path         string                 `json:"path"`
	Tag          string                 `json:"tag"`
	MockBodyJSON string                 `json:"mock_body_json,omitempty"`
	PathParams   []string               `json:"path_params,omitempty"`
	QueryParams  []Parameter            `json:"query_params,omitempty"`
	HeaderDoc    string                 `json:"header_doc,omitempty"`
	SchemaDoc    map[string]interface{} `json:"schema_doc,omitempty"`
}

// Schema represents an OpenAPI 3.0 schema
type Schema struct {
	Type        string             `json:"type" yaml:"type"`
	Format      string             `json:"format" yaml:"format"`
	Description string             `json:"description" yaml:"description"`
	Default     interface{}        `json:"default" yaml:"default"`
	Enum        []interface{}      `json:"enum" yaml:"enum"`
	Properties  map[string]*Schema `json:"properties" yaml:"properties"`
	Items       *Schema            `json:"items" yaml:"items"`
	Required    []string           `json:"required" yaml:"required"`
	Ref         string             `json:"$ref" yaml:"$ref"`
	Title       string             `json:"title" yaml:"title"`
	Example     interface{}        `json:"example" yaml:"example"`
}

// RawOpenAPI is the raw structure parsed from YAML
type RawOpenAPI struct {
	OpenAPI    string                          `yaml:"openapi"`
	Info       map[string]interface{}          `yaml:"info"`
	Paths      map[string]map[string]Operation `yaml:"paths"`
	Components struct {
		Schemas map[string]*Schema `yaml:"schemas"`
	} `yaml:"components"`
}

// TagGroup groups operations by tag/service
type TagGroup struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Count       int          `json:"count"`
	Endpoints   []*Operation `json:"endpoints"`
}

// ParsedSpec contains the parsed OpenAPI ready for the UI
type ParsedSpec struct {
	Version      string                `json:"version"`
	Title        string                `json:"title"`
	TotalCount   int                   `json:"total_count"`
	Tags         []string              `json:"tags"`
	Groups       []TagGroup            `json:"groups"`
	Endpoints    []*Operation          `json:"endpoints"`
	Schemas      map[string]*Schema    `json:"schemas"`
	EndpointMap  map[string]*Operation `json:"endpoint_map"` // key: "METHOD /path"
	LastModified string                `json:"last_modified"`
}
