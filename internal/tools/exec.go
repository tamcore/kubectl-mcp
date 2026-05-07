package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

const (
	defaultExecTimeout = 30 * time.Second
	maxExecTimeout     = 300 * time.Second
)

// ExecRunner abstracts the creation and execution of a remote command.
// In production, this calls remotecommand.NewSPDYExecutor; in tests, a fake.
type ExecRunner interface {
	Run(ctx context.Context, clientset kubernetes.Interface, cfg *rest.Config, namespace, pod, container string, command []string, stdout, stderr *bytes.Buffer) error
}

// spdyExecRunner is the production ExecRunner using SPDY.
type spdyExecRunner struct{}

func (s *spdyExecRunner) Run(ctx context.Context, clientset kubernetes.Interface, cfg *rest.Config, namespace, pod, container string, command []string, stdout, stderr *bytes.Buffer) error {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		Param("stdout", "true").
		Param("stderr", "true")

	for _, c := range command {
		req.Param("command", c)
	}
	if container != "" {
		req.Param("container", container)
	}

	executor, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("creating executor: %w", err)
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
	})
}

func registerExecPod(s *server.MCPServer, pool *kube.ClientPool, runner ExecRunner, cfg *config.Config) {
	if runner == nil {
		runner = &spdyExecRunner{}
	}

	tool := mcp.NewTool("exec_pod",
		mcp.WithDescription("Execute a command in a Kubernetes pod container. Requires --allow-write."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
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
		mcp.WithString("container",
			mcp.Description("Container name (defaults to first container)"),
		),
		// command can be a string array or a single string — handled via requireCommand.
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("Command to execute as a JSON array of strings (e.g. [\"ls\",\"-la\"]) or a single string"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Timeout in seconds (default 30, max 300)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctxName, err := pool.ResolveContext(req.GetString("context", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		cc, err := pool.ClientFor(ctxName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get client: %v", err)), nil
		}

		namespace, _ := req.RequireString("namespace")
		pod, _ := req.RequireString("pod")
		container := req.GetString("container", "")

		command, err := requireCommand(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		timeout := time.Duration(req.GetFloat("timeout", defaultExecTimeout.Seconds())) * time.Second
		if timeout > maxExecTimeout {
			timeout = maxExecTimeout
		}
		if timeout <= 0 {
			timeout = defaultExecTimeout
		}

		execCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		if err := applySafetyDelay(ctx, req, cfg.SafetyDelayWrite); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("safety delay interrupted: %v", err)), nil
		}

		var stdout, stderr bytes.Buffer
		err = runner.Run(execCtx, cc.Clientset, cc.RestConfig, namespace, pod, container, command, &stdout, &stderr)
		if err != nil {
			msg := formatExecError(err, timeout, &stderr)
			return mcp.NewToolResultError(msg), nil
		}

		var sb strings.Builder
		if stdout.Len() > 0 {
			sb.WriteString(stdout.String())
		}
		if stderr.Len() > 0 {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("STDERR:\n")
			sb.WriteString(stderr.String())
		}
		if sb.Len() == 0 {
			return mcp.NewToolResultText("(no output)"), nil
		}
		return mcp.NewToolResultText(sb.String()), nil
	})
}

// formatExecError builds an LLM-friendly error message for a failed exec.
// It detects deadline exceeded (timeout) and appends any stderr output.
func formatExecError(err error, timeout time.Duration, stderr *bytes.Buffer) string {
	var sb strings.Builder

	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintf(&sb,
			"exec failed: command timed out after %s. "+
				"The command may be long-running or interactive. "+
				"Use the timeout parameter (max %ds) to increase the limit.",
			timeout, int(maxExecTimeout.Seconds()))
	} else {
		fmt.Fprintf(&sb, "exec failed: %v", err)
	}

	if stderr != nil && stderr.Len() > 0 {
		sb.WriteString("\n\nSTDERR:\n")
		sb.WriteString(stderr.String())
	}

	return sb.String()
}

// requireCommand extracts the command parameter, handling both string arrays
// and single strings (LLMs may send either).
func requireCommand(req mcp.CallToolRequest) ([]string, error) {
	args := req.GetArguments()
	val, ok := args["command"]
	if !ok {
		return nil, fmt.Errorf("required argument \"command\" not found")
	}

	switch v := val.(type) {
	case string:
		// Some MCP clients stringify JSON arrays due to the WithString schema.
		// Detect and parse them before falling through to shell splitting.
		if cmd, ok := tryParseJSONArray(v); ok {
			return cmd, nil
		}
		parts, err := shellSplit(v)
		if err != nil {
			return nil, fmt.Errorf("invalid command string: %w", err)
		}
		if len(parts) == 0 {
			return nil, fmt.Errorf("command must not be empty")
		}
		return parts, nil
	case []any:
		cmd := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("command array elements must be strings")
			}
			cmd = append(cmd, s)
		}
		if len(cmd) == 0 {
			return nil, fmt.Errorf("command must not be empty")
		}
		return cmd, nil
	default:
		return nil, fmt.Errorf("command must be a string or array of strings")
	}
}

// tryParseJSONArray attempts to parse s as a JSON array of strings.
// Returns the parsed slice and true on success, or nil and false if s
// is not a valid JSON string array (in which case the caller should
// fall through to shell splitting).
func tryParseJSONArray(s string) ([]string, bool) {
	if len(s) < 2 || s[0] != '[' {
		return nil, false
	}
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil, false
	}
	if len(arr) == 0 {
		return nil, false
	}
	return arr, true
}
