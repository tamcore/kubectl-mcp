//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestRawAPI(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowRaw = true

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			t.Run("GET_healthz", func(t *testing.T) {
				result := callTool(t, c, "api_raw", map[string]any{
					"path": "/healthz",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "ok") {
					t.Errorf("expected 'ok' in /healthz response, got: %s", text)
				}
			})

			t.Run("GET_api_v1", func(t *testing.T) {
				result := callTool(t, c, "api_raw", map[string]any{
					"path": "/api/v1",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "resources") {
					t.Errorf("expected 'resources' in /api/v1 response, got: %.200s", text)
				}
			})

			t.Run("GET_api", func(t *testing.T) {
				result := callTool(t, c, "api_raw", map[string]any{
					"path": "/api",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "versions") {
					t.Errorf("expected 'versions' in /api response, got: %.200s", text)
				}
			})

			t.Run("POST_requires_allow_write", func(t *testing.T) {
				// Default config has AllowWrite=true, so create a server without it.
				cfgNoWrite := defaultConfig()
				cfgNoWrite.AllowRaw = true
				cfgNoWrite.AllowWrite = false
				cfgNoWrite.AllowDestructive = false

				base2 := tc.startFunc(t, cfgNoWrite)
				c2 := tc.clientFunc(t, base2)

				result := callTool(t, c2, "api_raw", map[string]any{
					"path":   "/api/v1/namespaces/default/configmaps",
					"method": "POST",
					"body":   `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"raw-test"}}`,
				})
				if !result.IsError {
					t.Error("expected error for POST without --allow-write")
				}
				text := resultText(result)
				if !strings.Contains(text, "--allow-write") {
					t.Errorf("expected --allow-write in error, got: %s", text)
				}
			})

			t.Run("invalid_path", func(t *testing.T) {
				result := callTool(t, c, "api_raw", map[string]any{
					"path": "no-leading-slash",
				})
				if !result.IsError {
					t.Error("expected error for path without leading /")
				}
			})
		})
	}
}

func TestRawAPI_NotRegistered(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowRaw = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "api_raw", map[string]any{
				"path": "/healthz",
			})
			if err == nil {
				t.Error("expected error when api_raw is not registered")
			}
		})
	}
}
