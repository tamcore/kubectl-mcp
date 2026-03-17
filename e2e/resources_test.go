//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestListResourceTemplates(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := c.ListResourceTemplates(ctx, mcp.ListResourceTemplatesRequest{})
			if err != nil {
				t.Fatalf("ListResourceTemplates: %v", err)
			}

			if len(result.ResourceTemplates) != 2 {
				t.Fatalf("expected 2 resource templates, got %d", len(result.ResourceTemplates))
			}

			names := make(map[string]bool)
			for _, tmpl := range result.ResourceTemplates {
				names[tmpl.Name] = true
			}

			if !names["Namespaced Kubernetes Resource"] {
				t.Error("missing namespaced resource template")
			}
			if !names["Cluster-scoped Kubernetes Resource"] {
				t.Error("missing cluster-scoped resource template")
			}
		})
	}
}

func TestReadResourceNamespaced(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			// Create a ConfigMap to read.
			cmName := "e2e-res-cm"
			manifest := configMapManifest(cmName, testNamespace, map[string]string{"hello": "world"})
			res := callTool(t, c, "apply_resource", map[string]any{
				"manifest": manifest,
			})
			if res.IsError {
				t.Fatalf("failed to create configmap: %s", resultText(res))
			}
			t.Cleanup(func() { deleteViaKubectl(t, "configmap", cmName, testNamespace) })

			// Determine the current context.
			ctxRes := callTool(t, c, "list_contexts", nil)
			ctxText := resultText(ctxRes)
			// Extract the first context name (the line starting with "- " or just the text).
			ctxName := extractCurrentContext(t, ctxText)

			// Read the ConfigMap via k8s:// URI.
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			uri := "k8s://" + ctxName + "/namespaces/" + testNamespace + "/core/v1/configmaps/" + cmName
			readReq := mcp.ReadResourceRequest{}
			readReq.Params.URI = uri

			readResult, err := c.ReadResource(ctx, readReq)
			if err != nil {
				t.Fatalf("ReadResource: %v", err)
			}

			if len(readResult.Contents) == 0 {
				t.Fatal("expected at least one content item")
			}

			textContents, ok := readResult.Contents[0].(mcp.TextResourceContents)
			if !ok {
				t.Fatalf("expected TextResourceContents, got %T", readResult.Contents[0])
			}

			if !strings.Contains(textContents.Text, "hello") {
				t.Errorf("expected configmap data in response, got: %s", textContents.Text)
			}
		})
	}
}

func TestReadResourceClusterScoped(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			// Get a node name.
			ctxName := extractCurrentContext(t, resultText(callTool(t, c, "list_contexts", nil)))
			nodeName := getFirstNodeName(t)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			uri := "k8s://" + ctxName + "/core/v1/nodes/" + nodeName
			readReq := mcp.ReadResourceRequest{}
			readReq.Params.URI = uri

			readResult, err := c.ReadResource(ctx, readReq)
			if err != nil {
				t.Fatalf("ReadResource: %v", err)
			}

			if len(readResult.Contents) == 0 {
				t.Fatal("expected at least one content item")
			}

			textContents, ok := readResult.Contents[0].(mcp.TextResourceContents)
			if !ok {
				t.Fatalf("expected TextResourceContents, got %T", readResult.Contents[0])
			}

			if !strings.Contains(textContents.Text, nodeName) {
				t.Errorf("expected node name in response, got: %s", textContents.Text)
			}
		})
	}
}

func TestReadResourceSecretRedaction(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowSecrets = false
			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			// Create a Secret.
			secretName := "e2e-res-secret"
			manifest := secretManifest(secretName, testNamespace, map[string]string{"token": "c2VjcmV0ZGF0YQ=="})
			res := callTool(t, c, "apply_resource", map[string]any{
				"manifest": manifest,
			})
			if res.IsError {
				t.Fatalf("failed to create secret: %s", resultText(res))
			}
			t.Cleanup(func() { deleteViaKubectl(t, "secret", secretName, testNamespace) })

			ctxName := extractCurrentContext(t, resultText(callTool(t, c, "list_contexts", nil)))

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			uri := "k8s://" + ctxName + "/namespaces/" + testNamespace + "/core/v1/secrets/" + secretName
			readReq := mcp.ReadResourceRequest{}
			readReq.Params.URI = uri

			readResult, err := c.ReadResource(ctx, readReq)
			if err != nil {
				t.Fatalf("ReadResource: %v", err)
			}

			textContents := readResult.Contents[0].(mcp.TextResourceContents)
			if strings.Contains(textContents.Text, "c2VjcmV0ZGF0YQ") {
				t.Error("secret data should be redacted")
			}
			if !strings.Contains(textContents.Text, "redacted") {
				t.Error("expected redacted placeholder")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractCurrentContext(t *testing.T, listContextsOutput string) string {
	t.Helper()
	// The list_contexts tool returns a JSON array of context objects.
	var contexts []map[string]interface{}
	if err := json.Unmarshal([]byte(listContextsOutput), &contexts); err != nil {
		// Fallback: try to parse as simple text.
		lines := strings.Split(strings.TrimSpace(listContextsOutput), "\n")
		if len(lines) > 0 {
			return strings.TrimSpace(strings.TrimPrefix(lines[0], "- "))
		}
		t.Fatalf("could not extract context from: %s", listContextsOutput)
	}
	if len(contexts) == 0 {
		t.Fatal("no contexts returned")
	}
	// Return the first context name.
	name, ok := contexts[0]["name"].(string)
	if !ok {
		t.Fatalf("could not extract context name from: %v", contexts[0])
	}
	return name
}

func getFirstNodeName(t *testing.T) string {
	t.Helper()
	out, err := kubectlOutput("get", "nodes", "-o", "jsonpath={.items[0].metadata.name}")
	if err != nil {
		t.Fatalf("getting node name: %v", err)
	}
	return strings.TrimSpace(out)
}
