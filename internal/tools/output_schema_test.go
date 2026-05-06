package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestStructuredToolsHaveOutputSchema(t *testing.T) {
	cfg := &config.Config{
		AllowedContexts:  []string{"*"},
		AllowWrite:       true,
		AllowDestructive: true,
	}
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
	RegisterAll(s, pool, cfg)

	tools := s.ListTools()

	structured := []string{"get_resource", "list_resources", "describe_resource", "rollout_status"}
	for _, name := range structured {
		tool, ok := tools[name]
		if !ok {
			t.Errorf("tool %q not found", name)
			continue
		}
		if tool.Tool.OutputSchema.Type == "" && len(tool.Tool.RawOutputSchema) == 0 {
			t.Errorf("tool %q: missing outputSchema declaration", name)
		}
	}
}
