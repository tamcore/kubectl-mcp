//go:build e2e

package e2e

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSecrets(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			// Create the secret via a write-enabled, secrets-hidden server.
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			secretName := "e2e-secret-" + strings.ToLower(tc.name)
			secret64 := base64.StdEncoding.EncodeToString([]byte("supersecret"))
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": secretManifest(secretName, testNamespace, map[string]string{"password": secret64}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "secret", secretName, testNamespace) })

			t.Run("get_redacted_by_default", func(t *testing.T) {
				result := callTool(t, c, "get_resource", map[string]any{
					"kind":      "Secret",
					"name":      secretName,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				// The response should contain <redacted> for data values.
				if !strings.Contains(text, "redacted") {
					// Log the actual response for debugging.
					t.Errorf("expected <redacted> in secret data, got:\n%s", text)
				}
				if strings.Contains(text, "supersecret") || strings.Contains(text, secret64) {
					t.Error("secret value leaked in response")
				}
			})

			t.Run("list_redacted_by_default", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "Secret",
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				// list_resources returns summaries, not full objects.
				// Just verify no leak of the actual secret value.
				if strings.Contains(text, "supersecret") {
					t.Error("secret value leaked in list response")
				}
			})

			t.Run("get_with_allow_secrets", func(t *testing.T) {
				// Start a separate server with AllowSecrets=true.
				cfg := defaultConfig()
				cfg.AllowSecrets = true
				base2 := tc.startFunc(t, cfg)
				c2 := tc.clientFunc(t, base2)

				result := callTool(t, c2, "get_resource", map[string]any{
					"kind":      "Secret",
					"name":      secretName,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if strings.Contains(text, "redacted") {
					t.Error("expected actual secret data with --allow-secrets, got <redacted>")
				}
				if !strings.Contains(text, secret64) {
					t.Error("expected base64-encoded secret value in response")
				}
			})
		})
	}
}

func TestDescribeResource_SecretRedaction(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			// Create the secret via a write-enabled server.
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			secretName := "e2e-describe-secret-" + strings.ToLower(tc.name)
			secret64 := base64.StdEncoding.EncodeToString([]byte("topsecret"))
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": secretManifest(secretName, testNamespace, map[string]string{"token": secret64}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "secret", secretName, testNamespace) })

			t.Run("describe_redacted_by_default", func(t *testing.T) {
				result := callTool(t, c, "describe_resource", map[string]any{
					"kind":      "Secret",
					"name":      secretName,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				// describe_resource returns both text and structuredContent.
				// The text format doesn't render the data section, but the
				// secret value must not leak anywhere in the response.
				if strings.Contains(text, "topsecret") || strings.Contains(text, secret64) {
					t.Error("secret value leaked in describe text response")
				}

				// Verify structuredContent has redacted data.
				if result.StructuredContent != nil {
					sc, ok := result.StructuredContent.(map[string]interface{})
					if ok {
						if data, ok := sc["data"].(map[string]interface{}); ok {
							for k, v := range data {
								if v != "<redacted>" {
									t.Errorf("expected data[%s] to be <redacted>, got %v", k, v)
								}
							}
						}
					}
				}
			})

			t.Run("describe_with_allow_secrets", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.AllowSecrets = true
				base2 := tc.startFunc(t, cfg)
				c2 := tc.clientFunc(t, base2)

				result := callTool(t, c2, "describe_resource", map[string]any{
					"kind":      "Secret",
					"name":      secretName,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}

				// With AllowSecrets, the structuredContent should contain the actual value.
				if result.StructuredContent != nil {
					sc, ok := result.StructuredContent.(map[string]interface{})
					if ok {
						if data, ok := sc["data"].(map[string]interface{}); ok {
							if data["token"] == "<redacted>" {
								t.Error("expected actual secret data with --allow-secrets, got <redacted>")
							}
						}
					}
				}
			})
		})
	}
}
