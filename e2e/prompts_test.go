//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestPrompts(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("list_prompts", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				result, err := c.ListPrompts(ctx, mcp.ListPromptsRequest{})
				if err != nil {
					t.Fatalf("ListPrompts: %v", err)
				}

				want := []string{"diagnose-pod", "diagnose-deployment", "investigate-node", "safe-rollback"}
				found := make(map[string]bool)
				for _, p := range result.Prompts {
					found[p.Name] = true
				}
				for _, name := range want {
					if !found[name] {
						t.Errorf("expected prompt %q in list, got: %v", name, result.Prompts)
					}
				}
			})

			t.Run("get_diagnose-pod", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				result, err := c.GetPrompt(ctx, mcp.GetPromptRequest{
					Params: mcp.GetPromptParams{
						Name:      "diagnose-pod",
						Arguments: map[string]string{"pod": "mypod", "namespace": testNamespace},
					},
				})
				if err != nil {
					t.Fatalf("GetPrompt diagnose-pod: %v", err)
				}
				if len(result.Messages) == 0 {
					t.Fatal("expected at least one message")
				}
				text := promptMessageText(result.Messages)
				for _, want := range []string{"mypod", testNamespace, "describe_resource", "get_events", "get_logs"} {
					if !strings.Contains(text, want) {
						t.Errorf("expected %q in prompt text, got: %s", want, text)
					}
				}
			})

			t.Run("get_diagnose-deployment", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				result, err := c.GetPrompt(ctx, mcp.GetPromptRequest{
					Params: mcp.GetPromptParams{
						Name:      "diagnose-deployment",
						Arguments: map[string]string{"deployment": "myapp", "namespace": testNamespace},
					},
				})
				if err != nil {
					t.Fatalf("GetPrompt diagnose-deployment: %v", err)
				}
				text := promptMessageText(result.Messages)
				for _, want := range []string{"myapp", testNamespace, "rollout_status", "describe_resource", "get_logs"} {
					if !strings.Contains(text, want) {
						t.Errorf("expected %q in prompt text, got: %s", want, text)
					}
				}
			})

			t.Run("get_investigate-node", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				result, err := c.GetPrompt(ctx, mcp.GetPromptRequest{
					Params: mcp.GetPromptParams{
						Name:      "investigate-node",
						Arguments: map[string]string{"node": "worker-0"},
					},
				})
				if err != nil {
					t.Fatalf("GetPrompt investigate-node: %v", err)
				}
				text := promptMessageText(result.Messages)
				for _, want := range []string{"worker-0", "describe_resource", "node_stats", "top_pods"} {
					if !strings.Contains(text, want) {
						t.Errorf("expected %q in prompt text, got: %s", want, text)
					}
				}
			})

			t.Run("get_safe-rollback", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				result, err := c.GetPrompt(ctx, mcp.GetPromptRequest{
					Params: mcp.GetPromptParams{
						Name:      "safe-rollback",
						Arguments: map[string]string{"deployment": "myapp", "namespace": testNamespace},
					},
				})
				if err != nil {
					t.Fatalf("GetPrompt safe-rollback: %v", err)
				}
				text := promptMessageText(result.Messages)
				for _, want := range []string{"myapp", testNamespace, "rollout_history", "rollout_undo", "rollout_status"} {
					if !strings.Contains(text, want) {
						t.Errorf("expected %q in prompt text, got: %s", want, text)
					}
				}
			})
		})
	}
}

func promptMessageText(msgs []mcp.PromptMessage) string {
	var sb strings.Builder
	for _, m := range msgs {
		if tc, ok := m.Content.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
