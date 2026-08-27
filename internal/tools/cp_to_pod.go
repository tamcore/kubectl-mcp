package tools

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// CopyRunner abstracts streaming a command with stdin into a pod.
// In production this uses SPDY; in tests a fake is injected.
type CopyRunner interface {
	Run(ctx context.Context, clientset kubernetes.Interface, cfg *rest.Config,
		namespace, pod, container string, command []string,
		stdin io.Reader, stdout, stderr *bytes.Buffer) error
}

// spdyCopyRunner is the production CopyRunner using SPDY with stdin support.
type spdyCopyRunner struct{}

func (r *spdyCopyRunner) Run(ctx context.Context, clientset kubernetes.Interface, cfg *rest.Config, namespace, pod, container string, command []string, stdin io.Reader, stdout, stderr *bytes.Buffer) error {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		Param("stdin", "true").
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
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
}

func registerCopyToPod(s *server.MCPServer, pool *kube.ClientPool, runner CopyRunner, cfg *config.Config) {
	if runner == nil {
		runner = &spdyCopyRunner{}
	}

	tool := mcp.NewTool("copy_to_pod",
		mcp.WithDescription("Copy a local file into a pod container (file-to-file, like kubectl cp). Requires --allow-write."),
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
		mcp.WithString("container",
			mcp.Description("Container name (defaults to first container)"),
		),
		mcp.WithString("dest_path",
			mcp.Required(),
			mcp.Description("Absolute destination path inside the container (e.g. /etc/myapp/config.yaml)"),
		),
		mcp.WithString("local_path",
			mcp.Required(),
			mcp.Description("Absolute path to a local file on the machine running the server to copy into the pod (file-to-file, like kubectl cp)"),
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

		namespace, err := req.RequireString("namespace")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		pod, err := req.RequireString("pod")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		destPath, err := req.RequireString("dest_path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !path.IsAbs(destPath) {
			return mcp.NewToolResultError("dest_path must be an absolute path (start with /)"), nil
		}
		if strings.Contains(destPath, "..") {
			return mcp.NewToolResultError("dest_path must not contain '..'"), nil
		}
		localPath, err := req.RequireString("local_path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !path.IsAbs(localPath) {
			return mcp.NewToolResultError("local_path must be an absolute path"), nil
		}
		container := req.GetString("container", "")

		info, err := os.Stat(localPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot read local_path: %v", err)), nil
		}
		if info.IsDir() {
			return mcp.NewToolResultError("local_path is a directory; only single files are supported"), nil
		}
		if info.Size() > maxCopyBytes {
			return mcp.NewToolResultError(
				fmt.Sprintf("local file exceeds maximum allowed size of %d MB", maxCopyBytes/(1024*1024)),
			), nil
		}

		data, err := os.ReadFile(localPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to read local file: %v", err)), nil
		}

		if err := applySafetyDelay(ctx, req, cfg.SafetyDelayWrite); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("safety delay interrupted: %v", err)), nil
		}

		tarBuf, err := buildTarBuffer(destPath, data)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to build tar archive: %v", err)), nil
		}

		var stdout, stderr bytes.Buffer
		if err := runner.Run(ctx, cc.Clientset, cc.RestConfig, namespace, pod, container,
			[]string{"tar", "xf", "-", "-C", "/"}, tarBuf, &stdout, &stderr); err != nil {
			msg := fmt.Sprintf("copy failed: %v", err)
			if stderr.Len() > 0 {
				msg += "\n\nSTDERR:\n" + stderr.String()
			}
			return mcp.NewToolResultError(msg), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("copied %d bytes to %s:%s", len(data), pod, destPath)), nil
	})
}

// buildTarBuffer creates an in-memory tar archive with a single file at destPath.
// The leading slash is stripped so tar extracts correctly under -C /.
func buildTarBuffer(destPath string, data []byte) (io.Reader, error) {
	name := strings.TrimPrefix(destPath, "/")

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(data)),
	}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(data); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}
