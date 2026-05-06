package prompts

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestDiagnosePodHandler(t *testing.T) {
	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name:      "diagnose-pod",
			Arguments: map[string]string{"pod": "myapp-abc", "namespace": "prod"},
		},
	}
	result, err := diagnosePodHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected at least one message")
	}
	text := messageText(result.Messages)
	for _, want := range []string{"myapp-abc", "prod", "describe_resource", "get_events", "get_logs"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in message text, got: %s", want, text)
		}
	}
}

func TestDiagnoseDeploymentHandler(t *testing.T) {
	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name:      "diagnose-deployment",
			Arguments: map[string]string{"deployment": "myapp", "namespace": "staging"},
		},
	}
	result, err := diagnoseDeploymentHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := messageText(result.Messages)
	for _, want := range []string{"myapp", "staging", "rollout_status", "describe_resource", "get_events", "get_logs"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in message text, got: %s", want, text)
		}
	}
}

func TestInvestigateNodeHandler(t *testing.T) {
	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name:      "investigate-node",
			Arguments: map[string]string{"node": "worker-1"},
		},
	}
	result, err := investigateNodeHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := messageText(result.Messages)
	for _, want := range []string{"worker-1", "describe_resource", "get_events", "node_stats", "top_pods"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in message text, got: %s", want, text)
		}
	}
}

func TestSafeRollbackHandler(t *testing.T) {
	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name:      "safe-rollback",
			Arguments: map[string]string{"deployment": "api-server", "namespace": "default"},
		},
	}
	result, err := safeRollbackHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := messageText(result.Messages)
	for _, want := range []string{"api-server", "default", "rollout_history", "rollout_undo", "rollout_status"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in message text, got: %s", want, text)
		}
	}
}

func TestHandlersMissingRequiredArgs(t *testing.T) {
	cases := []struct {
		name    string
		handler func(context.Context, mcp.GetPromptRequest) (*mcp.GetPromptResult, error)
		args    map[string]string
	}{
		{"diagnose-pod missing pod", diagnosePodHandler, map[string]string{"namespace": "default"}},
		{"diagnose-pod missing namespace", diagnosePodHandler, map[string]string{"pod": "mypod"}},
		{"diagnose-deployment missing deployment", diagnoseDeploymentHandler, map[string]string{"namespace": "default"}},
		{"diagnose-deployment missing namespace", diagnoseDeploymentHandler, map[string]string{"deployment": "myapp"}},
		{"investigate-node missing node", investigateNodeHandler, map[string]string{}},
		{"safe-rollback missing deployment", safeRollbackHandler, map[string]string{"namespace": "default"}},
		{"safe-rollback missing namespace", safeRollbackHandler, map[string]string{"deployment": "myapp"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := mcp.GetPromptRequest{
				Params: mcp.GetPromptParams{Arguments: tc.args},
			}
			_, err := tc.handler(context.Background(), req)
			if err == nil {
				t.Error("expected error for missing required argument, got nil")
			}
		})
	}
}

// messageText concatenates all text content from prompt messages.
func messageText(msgs []mcp.PromptMessage) string {
	var sb strings.Builder
	for _, m := range msgs {
		if tc, ok := m.Content.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
