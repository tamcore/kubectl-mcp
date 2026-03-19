//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestExplainResource(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("explain_pod", func(t *testing.T) {
				result := callTool(t, c, "explain_resource", map[string]any{
					"resource": "Pod",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				obj := jsonObjectFromResult(t, text)
				if obj["kind"] != "Pod" {
					t.Errorf("expected kind=Pod, got: %v", obj["kind"])
				}
				if obj["apiVersion"] != "v1" {
					t.Errorf("expected apiVersion=v1, got: %v", obj["apiVersion"])
				}
				if obj["namespaced"] != true {
					t.Errorf("expected namespaced=true, got: %v", obj["namespaced"])
				}
			})

			t.Run("explain_deployment", func(t *testing.T) {
				result := callTool(t, c, "explain_resource", map[string]any{
					"resource": "Deployment",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				obj := jsonObjectFromResult(t, text)
				if obj["kind"] != "Deployment" {
					t.Errorf("expected kind=Deployment, got: %v", obj["kind"])
				}
				// Deployment should have sub-resources like scale and status.
				subs, ok := obj["subResources"].([]any)
				if ok && len(subs) > 0 {
					t.Logf("sub-resources: %v", subs)
				}
			})

			t.Run("explain_namespace", func(t *testing.T) {
				result := callTool(t, c, "explain_resource", map[string]any{
					"resource": "Namespace",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				obj := jsonObjectFromResult(t, text)
				if obj["kind"] != "Namespace" {
					t.Errorf("expected kind=Namespace, got: %v", obj["kind"])
				}
				if obj["namespaced"] != false {
					t.Errorf("expected namespaced=false for Namespace, got: %v", obj["namespaced"])
				}
			})

			t.Run("explain_with_field_path", func(t *testing.T) {
				result := callTool(t, c, "explain_resource", map[string]any{
					"resource": "Deployment.spec.replicas",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				obj := jsonObjectFromResult(t, text)
				if obj["fieldPath"] != "spec.replicas" {
					t.Errorf("expected fieldPath=spec.replicas, got: %v", obj["fieldPath"])
				}
				// Should have field-level description from OpenAPI schema.
				if fd, ok := obj["fieldDescription"].(string); !ok || fd == "" {
					t.Errorf("expected non-empty fieldDescription, got: %v", obj["fieldDescription"])
				}
				if ft, ok := obj["fieldType"].(string); !ok || ft == "" {
					t.Errorf("expected non-empty fieldType, got: %v", obj["fieldType"])
				}
			})

			t.Run("explain_object_field_has_children", func(t *testing.T) {
				result := callTool(t, c, "explain_resource", map[string]any{
					"resource": "Deployment.spec.strategy",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				obj := jsonObjectFromResult(t, text)

				// strategy is an object type — should have child fields.
				fields, ok := obj["fields"].([]any)
				if !ok || len(fields) == 0 {
					t.Fatalf("expected non-empty fields array for object type, got: %v", obj["fields"])
				}

				// Should contain "type" and "rollingUpdate" as child fields.
				fieldNames := make(map[string]bool)
				for _, f := range fields {
					fm, _ := f.(map[string]any)
					if name, ok := fm["name"].(string); ok {
						fieldNames[name] = true
					}
				}
				if !fieldNames["type"] {
					t.Error("expected 'type' in child fields of spec.strategy")
				}
				if !fieldNames["rollingUpdate"] {
					t.Error("expected 'rollingUpdate' in child fields of spec.strategy")
				}
			})

			t.Run("explain_top_level_kind_has_fields", func(t *testing.T) {
				result := callTool(t, c, "explain_resource", map[string]any{
					"resource": "Deployment",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				obj := jsonObjectFromResult(t, text)

				// Top-level kind should have child fields (apiVersion, kind, metadata, spec, status).
				fields, ok := obj["fields"].([]any)
				if !ok || len(fields) == 0 {
					t.Fatalf("expected non-empty fields for top-level kind, got: %v", obj["fields"])
				}

				fieldNames := make(map[string]bool)
				for _, f := range fields {
					fm, _ := f.(map[string]any)
					if name, ok := fm["name"].(string); ok {
						fieldNames[name] = true
					}
				}
				if !fieldNames["spec"] {
					t.Error("expected 'spec' in top-level fields")
				}
				if !fieldNames["metadata"] {
					t.Error("expected 'metadata' in top-level fields")
				}
			})

			t.Run("unknown_resource_returns_error", func(t *testing.T) {
				result := callTool(t, c, "explain_resource", map[string]any{
					"resource": "NoSuchResource",
				})
				if !result.IsError {
					t.Error("expected error for unknown resource")
				}
				text := resultText(result)
				if text == "" {
					t.Error("expected error message")
				}
			})

			t.Run("explain_with_api_version", func(t *testing.T) {
				result := callTool(t, c, "explain_resource", map[string]any{
					"resource":   "Deployment",
					"apiVersion": "apps/v1",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				obj := jsonObjectFromResult(t, text)
				if !strings.Contains(obj["apiVersion"].(string), "apps") {
					t.Errorf("expected apiVersion containing 'apps', got: %v", obj["apiVersion"])
				}
			})
		})
	}
}
