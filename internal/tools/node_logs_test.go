package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	fakediscovery "k8s.io/client-go/discovery/fake"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// buildNodeLogsPool creates a pool for testing node_logs.
// The fake clientset doesn't fully support the proxy endpoint, but we can
// still test validation logic (path traversal, context not allowed, etc.).
func buildNodeLogsPool(cfg *config.Config) *kube.ClientPool {
	fakeCS := fake.NewClientset()
	disc := fakeCS.Discovery().(*fakediscovery.FakeDiscovery)
	disc.Resources = apiResources()

	return kube.NewClientPoolForTest(cfg, clientcmdapi.Config{
		CurrentContext: "test-ctx",
		Contexts:       map[string]*clientcmdapi.Context{"test-ctx": {}},
	}, map[string]*kube.ContextClient{
		"test-ctx": {
			Dynamic:   newFakeDynClient(),
			Clientset: fakeCS,
			Discovery: disc,
		},
	})
}

func TestNodeLogs_PathTraversal(t *testing.T) {
	cfg := defaultCfg()
	pool := buildNodeLogsPool(cfg)

	handler := getHandler(t, "node_logs", func(s *server.MCPServer) {
		registerNodeLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node":    "node-1",
		"logPath": "../etc/passwd",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error for path traversal attempt")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "path traversal") {
		t.Errorf("expected path traversal error, got: %s", text)
	}
}

func TestNodeLogs_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}}
	pool := buildNodeLogsPool(cfg)

	handler := getHandler(t, "node_logs", func(s *server.MCPServer) {
		registerNodeLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "node-1",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}

func TestNodeLogs_ValidatesLogPath(t *testing.T) {
	tests := []struct {
		name    string
		logPath string
		wantErr bool
		errMsg  string
	}{
		{"traversal double dot", "../secrets", true, "path traversal"},
		{"traversal mid path", "logs/../../etc", true, "path traversal"},
		{"absolute path", "/var/log/syslog", true, "path traversal"},
	}

	cfg := defaultCfg()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := buildNodeLogsPool(cfg)
			handler := getHandler(t, "node_logs", func(s *server.MCPServer) {
				registerNodeLogs(s, pool)
			})

			res, err := handler(context.Background(), callToolReq(map[string]any{
				"node":    "node-1",
				"logPath": tt.logPath,
			}))
			if err != nil {
				t.Fatal(err)
			}

			if tt.wantErr {
				if !res.IsError {
					t.Error("expected error")
				}
				text := resultText(t, res)
				if !strings.Contains(text, tt.errMsg) {
					t.Errorf("expected %q in error, got: %s", tt.errMsg, text)
				}
			}
		})
	}
}

func TestIsHTMLDirListing(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{"k3s HTML listing", `<!doctype html>
<meta name="viewport" content="width=device-width">
<pre>
<a href="94a8f22fe29e4261b8e2f12d13ab3c2a/">94a8f22fe29e4261b8e2f12d13ab3c2a/</a>
</pre>`, true},
		{"kubelet pre listing", `<pre>
<a href="syslog">syslog</a>
<a href="journal/">journal/</a>
</pre>`, true},
		{"plain text logs", "Mar 19 06:00:01 node kubelet[1234]: started\nMar 19 06:00:02 node kubelet[1234]: ready", false},
		{"empty", "", false},
		{"json output", `{"kind":"Status","apiVersion":"v1"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHTMLDirListing([]byte(tt.data))
			if got != tt.want {
				t.Errorf("isHTMLDirListing() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseHTMLDirLinks(t *testing.T) {
	html := `<!doctype html>
<meta name="viewport" content="width=device-width">
<pre>
<a href="94a8f22fe29e4261b8e2f12d13ab3c2a/">94a8f22fe29e4261b8e2f12d13ab3c2a/</a>
<a href="syslog">syslog</a>
<a href="messages">messages</a>
</pre>`

	links := parseHTMLDirLinks([]byte(html))
	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d: %v", len(links), links)
	}

	want := []string{"94a8f22fe29e4261b8e2f12d13ab3c2a/", "syslog", "messages"}
	for i, w := range want {
		if links[i] != w {
			t.Errorf("link[%d] = %q, want %q", i, links[i], w)
		}
	}
}

func TestParseHTMLDirLinks_Empty(t *testing.T) {
	links := parseHTMLDirLinks([]byte("no links here"))
	if len(links) != 0 {
		t.Errorf("expected 0 links, got %d", len(links))
	}
}

func TestFormatDirListing(t *testing.T) {
	links := []string{"journal/", "syslog", "messages"}
	result := formatDirListing("", links)
	if !strings.Contains(result, "directory listing") {
		t.Errorf("expected 'directory listing' in output, got: %s", result)
	}
	if !strings.Contains(result, "syslog") {
		t.Errorf("expected 'syslog' in output, got: %s", result)
	}

	// With parent path prefix.
	result = formatDirListing("journal", links)
	if !strings.Contains(result, "journal/syslog") {
		t.Errorf("expected 'journal/syslog' in output, got: %s", result)
	}
}

func TestValidateLogPath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"", false},
		{"syslog", false},
		{"journal/system.log", false},
		{"../etc/passwd", true},
		{"logs/../../etc", true},
		{"/var/log/syslog", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			err := validateLogPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLogPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}
