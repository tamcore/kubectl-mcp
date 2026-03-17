package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

const (
	defaultPortForwardTimeout = 300 * time.Second
	maxPortForwardTimeout     = 3600 * time.Second
)

// PortForwardRequest holds the parameters for a port-forward operation.
type PortForwardRequest struct {
	Namespace  string
	Pod        string
	RemotePort uint16
	LocalPort  uint16
	Timeout    time.Duration
}

// PortForwardResult holds the outcome of a successful port-forward.
type PortForwardResult struct {
	LocalPort uint16
	StopCh    chan struct{}
}

// PortForwarder abstracts the port-forward setup for testability.
type PortForwarder interface {
	Forward(ctx context.Context, req PortForwardRequest) (*PortForwardResult, error)
}

// activeForwards tracks live port-forward sessions for cleanup.
var activeForwards sync.Map

// portForwardResponse is the JSON response returned to the caller.
type portForwardResponse struct {
	Pod        string  `json:"pod"`
	Namespace  string  `json:"namespace"`
	LocalPort  uint16  `json:"localPort"`
	RemotePort uint16  `json:"remotePort"`
	Timeout    float64 `json:"timeout"`
	Message    string  `json:"message"`
}

// spdyPortForwarder is a placeholder production PortForwarder.
// A full SPDY-based implementation would use k8s.io/client-go/tools/portforward.
type spdyPortForwarder struct{}

func (s *spdyPortForwarder) Forward(_ context.Context, _ PortForwardRequest) (*PortForwardResult, error) {
	return nil, fmt.Errorf("SPDY port forwarding not yet implemented; use a test double")
}

func registerPortForward(s *server.MCPServer, pool *kube.ClientPool, forwarder PortForwarder) {
	if forwarder == nil {
		forwarder = &spdyPortForwarder{}
	}
	tool := mcp.NewTool("port_forward",
		mcp.WithDescription("Forward a local port to a pod port. Returns the local port number. Requires --allow-write."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace of the pod"),
		),
		mcp.WithString("pod",
			mcp.Required(),
			mcp.Description("Pod name"),
		),
		mcp.WithNumber("remotePort",
			mcp.Required(),
			mcp.Description("Port on the pod to forward to"),
		),
		mcp.WithNumber("localPort",
			mcp.Description("Local port to listen on (0 or omit for auto-assign)"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Timeout in seconds (default 300, max 3600). The port-forward will be closed after this duration."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctxName, err := pool.ResolveContext(req.GetString("context", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		_, err = pool.ClientFor(ctxName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get client: %v", err)), nil
		}

		namespace, _ := req.RequireString("namespace")
		pod, _ := req.RequireString("pod")
		remotePort := uint16(req.GetFloat("remotePort", 0))
		localPort := uint16(req.GetFloat("localPort", 0))
		timeout := clampTimeout(req.GetFloat("timeout", defaultPortForwardTimeout.Seconds()))

		if remotePort == 0 {
			return mcp.NewToolResultError("remotePort must be a valid port number (1-65535)"), nil
		}

		pfReq := PortForwardRequest{
			Namespace:  namespace,
			Pod:        pod,
			RemotePort: remotePort,
			LocalPort:  localPort,
			Timeout:    timeout,
		}

		result, err := forwarder.Forward(ctx, pfReq)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to start port forward: %v", err)), nil
		}

		// Store for cleanup and schedule timeout.
		key := fmt.Sprintf("%s/%s/%d", namespace, pod, result.LocalPort)
		activeForwards.Store(key, result.StopCh)

		go func() {
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case <-timer.C:
				close(result.StopCh)
				activeForwards.Delete(key)
			case <-result.StopCh:
				activeForwards.Delete(key)
			}
		}()

		resp := portForwardResponse{
			Pod:        pod,
			Namespace:  namespace,
			LocalPort:  result.LocalPort,
			RemotePort: remotePort,
			Timeout:    timeout.Seconds(),
			Message:    fmt.Sprintf("Port forward active: localhost:%d -> %s:%d (timeout: %s)", result.LocalPort, pod, remotePort, timeout),
		}

		out, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(out)), nil
	})
}

// clampTimeout converts a float seconds value to a clamped Duration.
func clampTimeout(seconds float64) time.Duration {
	timeout := time.Duration(seconds) * time.Second
	if timeout <= 0 {
		timeout = defaultPortForwardTimeout
	}
	if timeout > maxPortForwardTimeout {
		timeout = maxPortForwardTimeout
	}
	return timeout
}
