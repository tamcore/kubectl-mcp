package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// fakeRawRequester implements RawRequester for unit tests.
type fakeRawRequester struct {
	response []byte
	status   int
	err      error
}

func (f *fakeRawRequester) Do(_ context.Context, method, path, contentType string, body []byte) ([]byte, int, error) {
	return f.response, f.status, f.err
}

func rawHandler(t *testing.T, cfg *config.Config, requester RawRequester) server.ToolHandlerFunc {
	t.Helper()
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	return getHandler(t, "api_raw", func(s *server.MCPServer) {
		registerRawAPI(s, pool, cfg, requester)
	})
}

func TestRawAPI_GET_Success(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowRaw = true
	requester := &fakeRawRequester{response: []byte("ok"), status: 200}
	handler := rawHandler(t, cfg, requester)

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"path": "/healthz",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	text := resultText(t, res)
	if !strings.Contains(text, "ok") {
		t.Errorf("expected 'ok' in response, got: %s", text)
	}
}

func TestRawAPI_PathValidation(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowRaw = true
	requester := &fakeRawRequester{response: []byte("ok"), status: 200}
	handler := rawHandler(t, cfg, requester)

	t.Run("missing leading slash", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"path": "healthz",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error for path without leading /")
		}
	})

	t.Run("empty path", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"path": "",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error for empty path")
		}
	})

	t.Run("missing path", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error for missing path")
		}
	})

	t.Run("path too long", func(t *testing.T) {
		longPath := "/" + strings.Repeat("a", 2048)
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"path": longPath,
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error for path exceeding max length")
		}
	})
}

func TestRawAPI_InvalidMethod(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowRaw = true
	requester := &fakeRawRequester{response: []byte("ok"), status: 200}
	handler := rawHandler(t, cfg, requester)

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"path":   "/healthz",
		"method": "TRACE",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for invalid method")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "invalid method") {
		t.Errorf("expected 'invalid method' in error, got: %s", text)
	}
}

func TestRawAPI_NonGET_RequiresAllowWrite(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowRaw = true
	cfg.AllowWrite = false
	requester := &fakeRawRequester{response: []byte("ok"), status: 200}
	handler := rawHandler(t, cfg, requester)

	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			res, err := handler(context.Background(), callToolReq(map[string]any{
				"path":   "/api/v1/namespaces/default/configmaps",
				"method": method,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError {
				t.Errorf("expected error for %s without --allow-write", method)
			}
			text := resultText(t, res)
			if !strings.Contains(text, "--allow-write") {
				t.Errorf("expected error to mention --allow-write, got: %s", text)
			}
		})
	}
}

func TestRawAPI_POST_WithBody(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowRaw = true
	cfg.AllowWrite = true
	requester := &fakeRawRequester{response: []byte(`{"kind":"ConfigMap"}`), status: 201}
	handler := rawHandler(t, cfg, requester)

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"path":   "/api/v1/namespaces/default/configmaps",
		"method": "POST",
		"body":   `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	text := resultText(t, res)
	if !strings.Contains(text, "ConfigMap") {
		t.Errorf("expected ConfigMap in response, got: %s", text)
	}
}

func TestRawAPI_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other-ctx"}, AllowRaw: true}
	requester := &fakeRawRequester{response: []byte("ok"), status: 200}
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())

	handler := getHandler(t, "api_raw", func(s *server.MCPServer) {
		registerRawAPI(s, pool, cfg, requester)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"path": "/healthz",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for disallowed context")
	}
}

func TestRawAPI_APIError(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowRaw = true
	requester := &fakeRawRequester{
		response: nil,
		status:   404,
		err:      fmt.Errorf("the server could not find the requested resource"),
	}
	handler := rawHandler(t, cfg, requester)

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"path": "/api/v1/namespaces/nonexistent",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for API error")
	}
}

func TestRawAPI_MethodCaseInsensitive(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowRaw = true
	requester := &fakeRawRequester{response: []byte("ok"), status: 200}
	handler := rawHandler(t, cfg, requester)

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"path":   "/healthz",
		"method": "get",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
}

func TestRawAPI_ContentTypeValidation(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowRaw = true
	requester := &fakeRawRequester{response: []byte("ok"), status: 200}
	handler := rawHandler(t, cfg, requester)

	cases := []struct {
		name        string
		contentType string
	}{
		{"CR only", "application/json\r"},
		{"LF only", "application/json\n"},
		{"CRLF injection", "application/json\r\nX-Evil: injected"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := handler(context.Background(), callToolReq(map[string]any{
				"path":         "/api/v1/pods",
				"content_type": tc.contentType,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError {
				t.Errorf("expected error for content_type=%q", tc.contentType)
			}
			text := resultText(t, res)
			if !strings.Contains(text, "newline") {
				t.Errorf("expected newline error message, got: %s", text)
			}
		})
	}
}

func TestRawAPI_NotRegisteredWithoutAllowRaw(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowRaw = false
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())

	s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
	RegisterAll(s, pool, cfg)

	tool := s.GetTool("api_raw")
	if tool != nil {
		t.Error("api_raw should NOT be registered without --allow-raw")
	}
}

func TestRawAPI_RegisteredWithAllowRaw(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowRaw = true
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())

	s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
	RegisterAll(s, pool, cfg)

	tool := s.GetTool("api_raw")
	if tool == nil {
		t.Error("api_raw should be registered with --allow-raw")
	}
}
