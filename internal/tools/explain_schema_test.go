package tools

import "testing"

// Test schemas simulate a minimal OpenAPI v3 components.schemas structure.
func testSchemas() map[string]openAPISchema {
	return map[string]openAPISchema{
		"io.k8s.api.apps.v1.Deployment": {
			Description: "Deployment enables declarative updates for Pods and ReplicaSets.",
			Type:        "object",
			Properties: map[string]openAPISchema{
				"apiVersion": {Description: "APIVersion defines the versioned schema.", Type: "string"},
				"kind":       {Description: "Kind is a string value representing the REST resource.", Type: "string"},
				"metadata": {
					Description: "Standard object's metadata.",
					AllOf:       []openAPISchema{{Ref: "#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta"}},
				},
				"spec": {
					Description: "Specification of the desired behavior of the Deployment.",
					AllOf:       []openAPISchema{{Ref: "#/components/schemas/io.k8s.api.apps.v1.DeploymentSpec"}},
				},
			},
			XKubeGVK: []map[string]string{{"group": "apps", "version": "v1", "kind": "Deployment"}},
		},
		"io.k8s.api.apps.v1.DeploymentSpec": {
			Description: "DeploymentSpec is the specification of the desired behavior of the Deployment.",
			Type:        "object",
			Required:    []string{"selector", "template"},
			Properties: map[string]openAPISchema{
				"replicas": {Description: "Number of desired pods.", Type: "integer", Format: "int32"},
				"selector": {
					Description: "Label selector for pods.",
					AllOf:       []openAPISchema{{Ref: "#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector"}},
				},
				"strategy": {
					Description: "The deployment strategy to use to replace existing pods with new ones.",
					AllOf:       []openAPISchema{{Ref: "#/components/schemas/io.k8s.api.apps.v1.DeploymentStrategy"}},
				},
				"template": {Description: "Template describes the pods that will be created.", Type: "object"},
			},
		},
		"io.k8s.api.apps.v1.DeploymentStrategy": {
			Description: "DeploymentStrategy describes how to replace existing pods with new ones.",
			Type:        "object",
			Properties: map[string]openAPISchema{
				"type":          {Description: "Type of deployment. Can be \"Recreate\" or \"RollingUpdate\".", Type: "string"},
				"rollingUpdate": {Description: "Rolling update config params.", Type: "object"},
			},
		},
		"io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta": {
			Description: "ObjectMeta is metadata that all persisted resources must have.",
			Type:        "object",
			Properties: map[string]openAPISchema{
				"name":      {Description: "Name must be unique within a namespace.", Type: "string"},
				"namespace": {Description: "Namespace defines the space within which each name must be unique.", Type: "string"},
				"labels":    {Description: "Map of string keys and values.", Type: "object"},
			},
		},
		"io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector": {
			Description: "A label selector is a label query over a set of resources.",
			Type:        "object",
			Properties: map[string]openAPISchema{
				"matchLabels": {Description: "matchLabels is a map of key-value pairs.", Type: "object"},
			},
		},
	}
}

func TestOpenAPIPath(t *testing.T) {
	tests := []struct {
		group, version, want string
	}{
		{"", "v1", "api/v1"},
		{"apps", "v1", "apis/apps/v1"},
		{"batch", "v1", "apis/batch/v1"},
	}
	for _, tt := range tests {
		got := openAPIPath(tt.group, tt.version)
		if got != tt.want {
			t.Errorf("openAPIPath(%q, %q) = %q, want %q", tt.group, tt.version, got, tt.want)
		}
	}
}

func TestFindDefinitionKey(t *testing.T) {
	schemas := testSchemas()

	key := findDefinitionKey(schemas, "apps", "v1", "Deployment")
	if key != "io.k8s.api.apps.v1.Deployment" {
		t.Errorf("expected io.k8s.api.apps.v1.Deployment, got: %s", key)
	}

	key = findDefinitionKey(schemas, "apps", "v1", "NoSuchKind")
	if key != "" {
		t.Errorf("expected empty key for unknown kind, got: %s", key)
	}
}

func TestResolveSchema_Ref(t *testing.T) {
	schemas := testSchemas()

	ref := openAPISchema{Ref: "#/components/schemas/io.k8s.api.apps.v1.DeploymentSpec"}
	resolved := resolveSchema(ref, schemas)

	if resolved.Description != "DeploymentSpec is the specification of the desired behavior of the Deployment." {
		t.Errorf("unexpected description: %s", resolved.Description)
	}
	if len(resolved.Properties) == 0 {
		t.Error("expected properties after resolving ref")
	}
}

func TestResolveSchema_AllOf(t *testing.T) {
	schemas := testSchemas()

	s := openAPISchema{
		Description: "Spec of the deployment.",
		AllOf:       []openAPISchema{{Ref: "#/components/schemas/io.k8s.api.apps.v1.DeploymentSpec"}},
	}
	resolved := resolveSchema(s, schemas)

	// Parent description should be preserved.
	if resolved.Description != "Spec of the deployment." {
		t.Errorf("expected parent description, got: %s", resolved.Description)
	}
	if len(resolved.Properties) == 0 {
		t.Error("expected properties from allOf resolution")
	}
}

func TestSchemaTypeName(t *testing.T) {
	schemas := testSchemas()

	tests := []struct {
		name   string
		schema openAPISchema
		want   string
	}{
		{"string", openAPISchema{Type: "string"}, "string"},
		{"integer_format", openAPISchema{Type: "integer", Format: "int32"}, "integer (int32)"},
		{"ref", openAPISchema{Ref: "#/components/schemas/io.k8s.api.apps.v1.DeploymentStrategy"}, "DeploymentStrategy"},
		{"allOf_ref", openAPISchema{AllOf: []openAPISchema{{Ref: "#/components/schemas/io.k8s.api.apps.v1.DeploymentSpec"}}}, "DeploymentSpec"},
		{"array", openAPISchema{Type: "array", Items: &openAPISchema{Type: "string"}}, "[]string"},
		{"empty", openAPISchema{}, "object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schemaTypeName(tt.schema, schemas)
			if got != tt.want {
				t.Errorf("schemaTypeName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSchemaToFieldDetail_TopLevel(t *testing.T) {
	schemas := testSchemas()
	deploy := schemas["io.k8s.api.apps.v1.Deployment"]
	fd := schemaToFieldDetail(deploy, schemas)

	if fd.Description == "" {
		t.Error("expected non-empty description")
	}
	if len(fd.Fields) == 0 {
		t.Fatal("expected child fields for Deployment")
	}

	fieldNames := make(map[string]bool)
	for _, f := range fd.Fields {
		fieldNames[f.Name] = true
	}
	for _, want := range []string{"apiVersion", "kind", "metadata", "spec"} {
		if !fieldNames[want] {
			t.Errorf("expected field %q in Deployment", want)
		}
	}
}

func TestSchemaToFieldDetail_NestedField(t *testing.T) {
	schemas := testSchemas()

	// Resolve Deployment -> spec -> strategy
	deploy := schemas["io.k8s.api.apps.v1.Deployment"]
	specProp := deploy.Properties["spec"]
	specResolved := resolveSchema(specProp, schemas)
	strategyProp := specResolved.Properties["strategy"]
	strategyResolved := resolveSchema(strategyProp, schemas)

	fd := schemaToFieldDetail(strategyResolved, schemas)

	if fd.Description == "" {
		t.Error("expected non-empty description for strategy")
	}

	fieldNames := make(map[string]bool)
	for _, f := range fd.Fields {
		fieldNames[f.Name] = true
	}
	if !fieldNames["type"] {
		t.Error("expected 'type' in strategy fields")
	}
	if !fieldNames["rollingUpdate"] {
		t.Error("expected 'rollingUpdate' in strategy fields")
	}
}

func TestFirstSentence(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Hello world. More text.", "Hello world."},
		{"Single sentence", "Single sentence"},
		{"", ""},
		{"No period at end", "No period at end"},
	}
	for _, tt := range tests {
		got := firstSentence(tt.input)
		if got != tt.want {
			t.Errorf("firstSentence(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCoalesce(t *testing.T) {
	if got := coalesce("", "", "third"); got != "third" {
		t.Errorf("expected 'third', got %q", got)
	}
	if got := coalesce("first", "second"); got != "first" {
		t.Errorf("expected 'first', got %q", got)
	}
	if got := coalesce("", ""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
