//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestListRBACBindings(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("cluster_role_bindings", func(t *testing.T) {
				result := callTool(t, c, "list_rbac_bindings", map[string]any{})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				var items []map[string]interface{}
				if err := json.Unmarshal([]byte(text), &items); err != nil {
					t.Fatalf("expected JSON array, got: %s", text)
				}
				// KinD always has at least cluster-admin binding.
				if len(items) == 0 {
					t.Error("expected at least one ClusterRoleBinding")
				}
				for _, item := range items {
					if item["kind"] != "ClusterRoleBinding" {
						t.Errorf("expected kind=ClusterRoleBinding, got %v", item["kind"])
					}
				}
			})

			t.Run("role_bindings_in_namespace", func(t *testing.T) {
				result := callTool(t, c, "list_rbac_bindings", map[string]any{
					"namespace": "kube-system",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				// Must be valid JSON (may be empty array).
				var items []map[string]interface{}
				if err := json.Unmarshal([]byte(text), &items); err != nil {
					t.Fatalf("expected JSON array, got: %s", text)
				}
				for _, item := range items {
					if item["kind"] != "RoleBinding" {
						t.Errorf("expected kind=RoleBinding, got %v", item["kind"])
					}
				}
			})

			t.Run("subject_filter", func(t *testing.T) {
				result := callTool(t, c, "list_rbac_bindings", map[string]any{
					"subjectKind": "ServiceAccount",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				// Must be valid JSON.
				var items []map[string]interface{}
				if err := json.Unmarshal([]byte(text), &items); err != nil {
					t.Fatalf("expected JSON array, got: %s", text)
				}
			})
		})
	}
}

func TestListRBACRoles(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("cluster_roles", func(t *testing.T) {
				result := callTool(t, c, "list_rbac_roles", map[string]any{})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				var items []map[string]interface{}
				if err := json.Unmarshal([]byte(text), &items); err != nil {
					t.Fatalf("expected JSON array, got: %s", text)
				}
				if len(items) == 0 {
					t.Error("expected at least one ClusterRole")
				}
			})

			t.Run("named_cluster_role", func(t *testing.T) {
				result := callTool(t, c, "list_rbac_roles", map[string]any{
					"name": "cluster-admin",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				var obj map[string]interface{}
				if err := json.Unmarshal([]byte(text), &obj); err != nil {
					t.Fatalf("expected JSON object, got: %s", text)
				}
				if obj["name"] != "cluster-admin" {
					t.Errorf("expected name=cluster-admin, got %v", obj["name"])
				}
				if _, ok := obj["rules"]; !ok {
					t.Error("expected rules in detailed output")
				}
			})

			t.Run("roles_in_namespace", func(t *testing.T) {
				result := callTool(t, c, "list_rbac_roles", map[string]any{
					"namespace": "kube-system",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				// Must be valid JSON (may be empty array).
				var items []map[string]interface{}
				if err := json.Unmarshal([]byte(text), &items); err != nil {
					t.Fatalf("expected JSON array, got: %s", text)
				}
			})

			t.Run("not_found_returns_error", func(t *testing.T) {
				result := callTool(t, c, "list_rbac_roles", map[string]any{
					"name": "nonexistent-role-xyz",
				})
				if !result.IsError {
					t.Error("expected error for nonexistent role")
				}
			})
		})
	}
}

func TestListServiceAccounts(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("list_in_namespace", func(t *testing.T) {
				result := callTool(t, c, "list_service_accounts", map[string]any{
					"namespace": "default",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				var items []map[string]interface{}
				if err := json.Unmarshal([]byte(text), &items); err != nil {
					t.Fatalf("expected JSON array, got: %s", text)
				}
				// KinD default namespace always has the "default" SA.
				if len(items) == 0 {
					t.Error("expected at least one ServiceAccount in default namespace")
				}
			})

			t.Run("named_sa_shows_secret_names", func(t *testing.T) {
				result := callTool(t, c, "list_service_accounts", map[string]any{
					"namespace": "default",
					"name":      "default",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				var obj map[string]interface{}
				if err := json.Unmarshal([]byte(text), &obj); err != nil {
					t.Fatalf("expected JSON object, got: %s", text)
				}
				if obj["name"] != "default" {
					t.Errorf("expected name=default, got %v", obj["name"])
				}
				// secrets key must exist (may be empty slice).
				if _, ok := obj["secrets"]; !ok {
					t.Error("expected secrets key in SA detail output")
				}
				// No raw token data should be exposed.
				if strings.Contains(text, `"data"`) {
					t.Error("secret data must never be exposed in SA output")
				}
			})

			t.Run("all_namespaces", func(t *testing.T) {
				result := callTool(t, c, "list_service_accounts", map[string]any{})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				var items []map[string]interface{}
				if err := json.Unmarshal([]byte(text), &items); err != nil {
					t.Fatalf("expected JSON array, got: %s", text)
				}
				if len(items) == 0 {
					t.Error("expected service accounts across all namespaces")
				}
			})
		})
	}
}
