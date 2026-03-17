package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestAllToolsHaveAnnotations(t *testing.T) {
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

	readOnlyTools := map[string]bool{
		"list_contexts": true, "list_namespaces": true, "list_api_resources": true,
		"get_resource": true, "list_resources": true, "describe_resource": true,
		"get_logs": true, "get_events": true, "top_pods": true, "top_nodes": true,
	}

	destructiveTools := map[string]bool{
		"delete_resource": true, "drain_node": true, "exec_pod": true,
	}

	for name, tool := range tools {
		ann := tool.Tool.Annotations
		if ann.ReadOnlyHint == nil {
			t.Errorf("tool %q missing ReadOnlyHint", name)
			continue
		}
		if ann.DestructiveHint == nil {
			t.Errorf("tool %q missing DestructiveHint", name)
			continue
		}
		if ann.IdempotentHint == nil {
			t.Errorf("tool %q missing IdempotentHint", name)
			continue
		}
		if ann.OpenWorldHint == nil {
			t.Errorf("tool %q missing OpenWorldHint", name)
			continue
		}

		if readOnlyTools[name] {
			if !*ann.ReadOnlyHint {
				t.Errorf("tool %q: expected ReadOnlyHint=true", name)
			}
			if *ann.DestructiveHint {
				t.Errorf("tool %q: expected DestructiveHint=false for read-only tool", name)
			}
		}

		if destructiveTools[name] {
			if *ann.ReadOnlyHint {
				t.Errorf("tool %q: expected ReadOnlyHint=false for destructive tool", name)
			}
			if !*ann.DestructiveHint {
				t.Errorf("tool %q: expected DestructiveHint=true for destructive tool", name)
			}
		}
	}
}
