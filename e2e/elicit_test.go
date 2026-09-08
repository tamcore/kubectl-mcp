//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestElicitation_GracefulDegradation verifies that destructive tools work
// without elicitation support in the client (graceful degradation).
// The SSE/HTTP clients in this test suite do not support elicitation,
// so the server should proceed without confirmation.
func TestElicitation_GracefulDegradation(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			// Create a resource to delete.
			name := "e2e-elicit-" + strings.ToLower(tc.name)
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": configMapManifest(name, testNamespace, map[string]string{"k": "v"}),
			})

			// Delete should succeed despite no elicitation support in the client.
			result := callTool(t, c, "delete_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      name,
				"namespace": testNamespace,
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("expected delete to succeed (graceful degradation), got error: %s", text)
			}
			if !strings.Contains(text, "Deleted ConfigMap/"+name) {
				t.Errorf("expected delete confirmation, got: %s", text)
			}
		})
	}
}

// TestElicitation_DryRunSkipsConfirmation verifies that dry-run deletes
// skip the elicitation confirmation.
func TestElicitation_DryRunSkipsConfirmation(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			name := "e2e-elicit-dry-" + strings.ToLower(tc.name)
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": configMapManifest(name, testNamespace, map[string]string{"k": "v"}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

			result := callTool(t, c, "delete_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      name,
				"namespace": testNamespace,
				"dryRun":    true,
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("error: %s", text)
			}
			if !strings.Contains(text, "DRY RUN") {
				t.Errorf("expected DRY RUN in output, got: %s", text)
			}
		})
	}
}

// confirmHandler answers every elicitation with a fixed confirmation.
type confirmHandler struct {
	confirm bool
}

func (h confirmHandler) Elicit(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	return &mcp.ElicitationResult{
		Action:  mcp.ElicitationResponseActionAccept,
		Content: map[string]any{"confirm": h.confirm},
	}, nil
}

// newConfirmingClient starts a streamable HTTP client that declares the
// elicitation capability, negotiates the given protocol version, and answers
// confirmations with confirm.
func newConfirmingClient(t *testing.T, base, protocolVersion string, confirm bool) *mcpclient.Client {
	t.Helper()

	trans, err := transport.NewStreamableHTTP(base + "/mcp")
	if err != nil {
		t.Fatalf("NewStreamableHTTP: %v", err)
	}
	c := mcpclient.NewClient(trans, mcpclient.WithElicitationHandler(confirmHandler{confirm: confirm}))
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = protocolVersion
	initReq.Params.ClientInfo = mcp.Implementation{Name: "e2e-elicit", Version: "0.0.1"}
	initReq.Params.Capabilities.Elicitation = new(mcp.ElicitationCapability)

	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

// TestElicitation_ConfirmedDelete verifies that a client which declares the
// elicitation capability confirms the delete, on both the legacy
// server-initiated path and the multi round-trip path introduced in protocol
// version 2026-07-28.
func TestElicitation_ConfirmedDelete(t *testing.T) {
	protocolVersions := []string{"2024-11-05", mcp.ProtocolVersion20260728}

	for _, version := range protocolVersions {
		t.Run(version, func(t *testing.T) {
			base := startStreamableHTTPServerWithConfig(t, defaultConfig())
			admin := newHTTPClient(t, base)
			c := newConfirmingClient(t, base, version, true)

			name := "e2e-elicit-confirm-" + strings.ReplaceAll(version, "-", "")
			callTool(t, admin, "apply_resource", map[string]any{
				"manifest": configMapManifest(name, testNamespace, map[string]string{"k": "v"}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

			result := callTool(t, c, "delete_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      name,
				"namespace": testNamespace,
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("expected confirmed delete to succeed, got error: %s", text)
			}
			if !strings.Contains(text, "Deleted ConfigMap/"+name) {
				t.Errorf("expected delete confirmation, got: %s", text)
			}
		})
	}
}

// TestElicitation_DeclinedDelete verifies that a declined confirmation leaves
// the resource untouched.
func TestElicitation_DeclinedDelete(t *testing.T) {
	protocolVersions := []string{"2024-11-05", mcp.ProtocolVersion20260728}

	for _, version := range protocolVersions {
		t.Run(version, func(t *testing.T) {
			base := startStreamableHTTPServerWithConfig(t, defaultConfig())
			admin := newHTTPClient(t, base)
			c := newConfirmingClient(t, base, version, false)

			name := "e2e-elicit-decline-" + strings.ReplaceAll(version, "-", "")
			callTool(t, admin, "apply_resource", map[string]any{
				"manifest": configMapManifest(name, testNamespace, map[string]string{"k": "v"}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

			result := callTool(t, c, "delete_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      name,
				"namespace": testNamespace,
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("expected declined delete to return a message, got error: %s", text)
			}
			if !strings.Contains(text, "cancelled by user") {
				t.Errorf("expected cancellation message, got: %s", text)
			}

			get := callTool(t, admin, "get_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      name,
				"namespace": testNamespace,
			})
			if get.IsError {
				t.Errorf("expected the ConfigMap to still exist, got: %s", resultText(get))
			}
		})
	}
}
