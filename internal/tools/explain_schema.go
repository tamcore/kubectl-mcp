package tools

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"

	"k8s.io/client-go/discovery"
)

// fieldDetail holds schema information for a specific field.
type fieldDetail struct {
	Description string      `json:"description,omitempty"`
	Type        string      `json:"type,omitempty"`
	Fields      []fieldInfo `json:"fields,omitempty"`
}

// fieldInfo describes a single child field.
type fieldInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// openAPISchema is a minimal representation of an OpenAPI v3 schema object.
type openAPISchema struct {
	Description string                   `json:"description,omitempty"`
	Type        string                   `json:"type,omitempty"`
	Ref         string                   `json:"$ref,omitempty"`
	AllOf       []openAPISchema          `json:"allOf,omitempty"`
	Properties  map[string]openAPISchema `json:"properties,omitempty"`
	Required    []string                 `json:"required,omitempty"`
	Items       *openAPISchema           `json:"items,omitempty"`
	Format      string                   `json:"format,omitempty"`
	XKubeGVK    []map[string]string      `json:"x-kubernetes-group-version-kind,omitempty"`
	Components  *openAPIComponents       `json:"-"` // set after unmarshal for ref resolution
}

// openAPIComponents holds the schemas section of an OpenAPI v3 document.
type openAPIComponents struct {
	Schemas map[string]openAPISchema `json:"schemas"`
}

// openAPIDoc represents the top-level OpenAPI v3 document.
type openAPIDoc struct {
	Components openAPIComponents `json:"components"`
}

// fetchFieldDetail fetches the OpenAPI v3 schema for a resource and resolves the field path.
func fetchFieldDetail(disc discovery.DiscoveryInterface, group, version, kind, fieldPath string) (fd *fieldDetail, err error) {
	// Guard against discovery implementations that panic on OpenAPIV3 (e.g., fake clients in tests).
	defer func() {
		if r := recover(); r != nil {
			fd = nil
			err = fmt.Errorf("OpenAPI v3 not supported: %v", r)
		}
	}()

	schemaPath := openAPIPath(group, version)

	paths, err := disc.OpenAPIV3().Paths()
	if err != nil {
		return nil, fmt.Errorf("fetching OpenAPI paths: %w", err)
	}

	gv, ok := paths[schemaPath]
	if !ok {
		return nil, fmt.Errorf("OpenAPI schema not found for %s", schemaPath)
	}

	raw, err := gv.Schema("application/json")
	if err != nil {
		return nil, fmt.Errorf("fetching OpenAPI schema: %w", err)
	}

	var doc openAPIDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing OpenAPI schema: %w", err)
	}

	defKey := findDefinitionKey(doc.Components.Schemas, group, version, kind)
	if defKey == "" {
		return nil, fmt.Errorf("schema definition not found for %s in %s/%s", kind, group, version)
	}

	rootSchema := doc.Components.Schemas[defKey]
	resolved := resolveSchema(rootSchema, doc.Components.Schemas)

	if fieldPath == "" {
		return schemaToFieldDetail(resolved, doc.Components.Schemas), nil
	}

	// Walk the field path.
	parts := strings.Split(fieldPath, ".")
	current := resolved
	for _, part := range parts {
		props := resolveProperties(current, doc.Components.Schemas)
		child, ok := props[part]
		if !ok {
			return nil, fmt.Errorf("field %q not found in schema", fieldPath)
		}
		current = resolveSchema(child, doc.Components.Schemas)
	}

	return schemaToFieldDetail(current, doc.Components.Schemas), nil
}

// openAPIPath returns the OpenAPI v3 path for a group/version.
func openAPIPath(group, version string) string {
	if group == "" {
		return "api/" + version
	}
	return "apis/" + group + "/" + version
}

// findDefinitionKey finds the schema key matching the given GVK.
func findDefinitionKey(schemas map[string]openAPISchema, group, version, kind string) string {
	for key, schema := range schemas {
		for _, gvk := range schema.XKubeGVK {
			if gvk["group"] == group && gvk["version"] == version && gvk["kind"] == kind {
				return key
			}
		}
	}
	return ""
}

// resolveSchema resolves $ref and allOf to get the actual schema.
func resolveSchema(s openAPISchema, schemas map[string]openAPISchema) openAPISchema {
	if s.Ref != "" {
		key := strings.TrimPrefix(s.Ref, "#/components/schemas/")
		if resolved, ok := schemas[key]; ok {
			// Preserve the parent description if the ref target has none.
			if s.Description != "" && resolved.Description == "" {
				resolved.Description = s.Description
			}
			return resolveSchema(resolved, schemas)
		}
	}
	if len(s.AllOf) > 0 {
		merged := openAPISchema{
			Description: s.Description,
			Type:        s.Type,
			Properties:  s.Properties,
			Required:    s.Required,
		}
		for _, part := range s.AllOf {
			resolved := resolveSchema(part, schemas)
			if merged.Description == "" {
				merged.Description = resolved.Description
			}
			if merged.Type == "" {
				merged.Type = resolved.Type
			}
			if resolved.Properties != nil {
				if merged.Properties == nil {
					merged.Properties = make(map[string]openAPISchema)
				}
				maps.Copy(merged.Properties, resolved.Properties)
			}
			if resolved.Required != nil {
				merged.Required = append(merged.Required, resolved.Required...)
			}
			if resolved.Items != nil && merged.Items == nil {
				merged.Items = resolved.Items
			}
		}
		return merged
	}
	return s
}

// resolveProperties returns the properties of a schema, following refs.
func resolveProperties(s openAPISchema, schemas map[string]openAPISchema) map[string]openAPISchema {
	resolved := resolveSchema(s, schemas)
	return resolved.Properties
}

// schemaToFieldDetail converts an openAPISchema to a fieldDetail.
func schemaToFieldDetail(s openAPISchema, schemas map[string]openAPISchema) *fieldDetail {
	fd := &fieldDetail{
		Description: s.Description,
		Type:        schemaTypeName(s, schemas),
	}

	requiredSet := make(map[string]bool, len(s.Required))
	for _, r := range s.Required {
		requiredSet[r] = true
	}

	if s.Properties != nil {
		names := make([]string, 0, len(s.Properties))
		for name := range s.Properties {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			prop := s.Properties[name]
			resolved := resolveSchema(prop, schemas)
			fi := fieldInfo{
				Name:        name,
				Type:        schemaTypeName(resolved, schemas),
				Description: firstSentence(coalesce(prop.Description, resolved.Description)),
				Required:    requiredSet[name],
			}
			fd.Fields = append(fd.Fields, fi)
		}
	}

	return fd
}

// schemaTypeName extracts a human-readable type name from a schema.
func schemaTypeName(s openAPISchema, schemas map[string]openAPISchema) string {
	if s.Ref != "" {
		key := strings.TrimPrefix(s.Ref, "#/components/schemas/")
		parts := strings.Split(key, ".")
		return parts[len(parts)-1]
	}
	if len(s.AllOf) > 0 {
		for _, part := range s.AllOf {
			if part.Ref != "" {
				return schemaTypeName(part, schemas)
			}
		}
	}
	if s.Type == "array" && s.Items != nil {
		itemType := schemaTypeName(*s.Items, schemas)
		return "[]" + itemType
	}
	if s.Type != "" {
		if s.Format != "" {
			return s.Type + " (" + s.Format + ")"
		}
		return s.Type
	}
	return "object"
}

// firstSentence returns the first sentence of a string (up to the first period+space or end).
func firstSentence(s string) string {
	if idx := strings.Index(s, ". "); idx >= 0 {
		return s[:idx+1]
	}
	return s
}

// coalesce returns the first non-empty string.
func coalesce(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
