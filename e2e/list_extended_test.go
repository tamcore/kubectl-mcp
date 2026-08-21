//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestListResourcesWithLimit(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			suffix := strings.ToLower(tc.name)

			// Create 3 ConfigMaps.
			names := []string{
				"e2e-list-lim-a-" + suffix,
				"e2e-list-lim-b-" + suffix,
				"e2e-list-lim-c-" + suffix,
			}
			for _, name := range names {
				manifest := configMapManifest(name, testNamespace, map[string]string{"k": "v"})
				callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
			}
			t.Cleanup(func() {
				for _, name := range names {
					deleteViaKubectl(t, "configmap", name, testNamespace)
				}
			})

			t.Run("limit_2", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "ConfigMap",
					"namespace": testNamespace,
					"limit":     float64(2),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				items := jsonArrayFromResult(t, text)
				if len(items) > 2 {
					t.Errorf("expected at most 2 items with limit=2, got %d", len(items))
				}
				if strings.Contains(text, "continue=") {
					t.Log("pagination token present as expected")
				}
			})
		})
	}
}

func TestListResourcesWithFieldSelector(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("field_selector_status_phase", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":          "Pod",
					"namespace":     "kube-system",
					"fieldSelector": "status.phase=Running",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				items := jsonArrayFromResult(t, text)
				if len(items) == 0 {
					t.Error("expected at least one running pod in kube-system")
				}
				for _, item := range items {
					status, _ := item["status"].(string)
					if status != "" && status != "Running" {
						t.Errorf("expected Running pod, got status=%s", status)
					}
				}
			})

			t.Run("field_selector_by_name", func(t *testing.T) {
				suffix := strings.ToLower(tc.name)
				name := "e2e-list-fs-" + suffix
				manifest := configMapManifest(name, testNamespace, map[string]string{"k": "v"})
				callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
				t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

				result := callTool(t, c, "list_resources", map[string]any{
					"kind":          "ConfigMap",
					"namespace":     testNamespace,
					"fieldSelector": "metadata.name=" + name,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				items := jsonArrayFromResult(t, text)
				if len(items) != 1 {
					t.Errorf("expected exactly 1 ConfigMap, got %d", len(items))
				}
				if len(items) > 0 {
					if items[0]["name"] != name {
						t.Errorf("expected name=%s, got: %v", name, items[0]["name"])
					}
				}
			})
		})
	}
}

func TestListResourcesAllNamespaces(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			suffix := strings.ToLower(tc.name)

			// Create ConfigMaps in two different namespaces.
			nameA := "e2e-all-ns-a-" + suffix
			nameB := "e2e-all-ns-b-" + suffix
			manifestA := configMapManifest(nameA, testNamespace, map[string]string{"k": "v"})
			manifestB := configMapManifest(nameB, "kube-system", map[string]string{"k": "v"})
			callTool(t, c, "apply_resource", map[string]any{"manifest": manifestA})
			callTool(t, c, "apply_resource", map[string]any{"manifest": manifestB})
			t.Cleanup(func() {
				deleteViaKubectl(t, "configmap", nameA, testNamespace)
				deleteViaKubectl(t, "configmap", nameB, "kube-system")
			})

			t.Run("list_all_namespaces", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":          "ConfigMap",
					"allNamespaces": true,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, nameA) {
					t.Errorf("expected ConfigMap %s from %s namespace", nameA, testNamespace)
				}
				if !strings.Contains(text, nameB) {
					t.Errorf("expected ConfigMap %s from kube-system namespace", nameB)
				}
			})

			t.Run("all_namespaces_with_namespace_errors", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":          "ConfigMap",
					"allNamespaces": true,
					"namespace":     testNamespace,
				})
				if !result.IsError {
					t.Error("expected error when allNamespaces=true and namespace is set")
				}
			})
		})
	}
}

func TestListResourcesSortBy(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			suffix := strings.ToLower(tc.name)

			// Create 3 ConfigMaps with distinct names so ordering is predictable.
			names := []string{
				"e2e-sort-charlie-" + suffix,
				"e2e-sort-alpha-" + suffix,
				"e2e-sort-bravo-" + suffix,
			}
			for _, name := range names {
				manifest := configMapManifest(name, testNamespace, map[string]string{"k": "v"})
				callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
			}
			t.Cleanup(func() {
				for _, name := range names {
					deleteViaKubectl(t, "configmap", name, testNamespace)
				}
			})

			t.Run("sort_by_name", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "ConfigMap",
					"namespace": testNamespace,
					"sortBy":    ".metadata.name",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				items := jsonArrayFromResult(t, text)
				var ourNames []string
				for _, item := range items {
					if n, ok := item["name"].(string); ok {
						if strings.HasPrefix(n, "e2e-sort-") && strings.HasSuffix(n, "-"+suffix) {
							ourNames = append(ourNames, n)
						}
					}
				}
				if len(ourNames) < 3 {
					t.Fatalf("expected at least 3 sort test ConfigMaps, got %d: %v", len(ourNames), ourNames)
				}
				for i := 1; i < len(ourNames); i++ {
					if ourNames[i-1] > ourNames[i] {
						t.Errorf("not sorted ascending: %s > %s", ourNames[i-1], ourNames[i])
					}
				}
			})

			t.Run("sort_descending", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "ConfigMap",
					"namespace": testNamespace,
					"sortBy":    "-.metadata.name",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				items := jsonArrayFromResult(t, text)
				var ourNames []string
				for _, item := range items {
					if n, ok := item["name"].(string); ok {
						if strings.HasPrefix(n, "e2e-sort-") && strings.HasSuffix(n, "-"+suffix) {
							ourNames = append(ourNames, n)
						}
					}
				}
				if len(ourNames) < 3 {
					t.Fatalf("expected at least 3 sort test ConfigMaps, got %d: %v", len(ourNames), ourNames)
				}
				for i := 1; i < len(ourNames); i++ {
					if ourNames[i-1] < ourNames[i] {
						t.Errorf("not sorted descending: %s < %s", ourNames[i-1], ourNames[i])
					}
				}
			})
		})
	}
}

func TestListResourcesWithLabelSelector(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "list_resources", map[string]any{
				"kind":          "Pod",
				"namespace":     "kube-system",
				"labelSelector": "component=kube-apiserver",
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("error: %s", text)
			}
			items := jsonArrayFromResult(t, text)
			if len(items) == 0 {
				t.Error("expected at least one kube-apiserver pod")
			}
		})
	}
}
