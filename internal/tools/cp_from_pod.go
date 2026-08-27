package tools

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerCopyFromPod(s *server.MCPServer, pool *kube.ClientPool, runner ExecRunner, cfg *config.Config) {
	if runner == nil {
		runner = &spdyExecRunner{}
	}

	tool := mcp.NewTool("copy_from_pod",
		mcp.WithDescription("Copy a file from a pod container. Returns contents by default (text as-is, binary base64-encoded); if local_path is set, writes the file there instead (use overwrite=true to replace an existing file)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
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
		mcp.WithString("src_path",
			mcp.Required(),
			mcp.Description("Absolute path of the file to copy from the container"),
		),
		mcp.WithString("local_path",
			mcp.Description("Absolute local path to write the file to. If set, the file is saved there instead of being returned inline."),
		),
		mcp.WithBoolean("overwrite",
			mcp.Description("Overwrite the local file if it already exists (only used with local_path)"),
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
		srcPath, err := req.RequireString("src_path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		container := req.GetString("container", "")
		localPath := req.GetString("local_path", "")
		overwrite := req.GetBool("overwrite", false)

		if localPath != "" {
			if !path.IsAbs(localPath) {
				return mcp.NewToolResultError("local_path must be an absolute path"), nil
			}
			if !overwrite {
				if _, statErr := os.Stat(localPath); statErr == nil {
					return mcp.NewToolResultError(
						fmt.Sprintf("local file %s already exists; set overwrite=true to replace it", localPath),
					), nil
				}
			}
		}

		var stdout, stderr bytes.Buffer
		if err := runner.Run(ctx, cc.Clientset, cc.RestConfig, namespace, pod, container,
			[]string{"tar", "cf", "-", srcPath}, &stdout, &stderr); err != nil {
			msg := fmt.Sprintf("copy failed: %v", err)
			if stderr.Len() > 0 {
				msg += "\n\nSTDERR:\n" + stderr.String()
			}
			return mcp.NewToolResultError(msg), nil
		}

		content, err := extractTarFile(&stdout)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to read tar stream: %v", err)), nil
		}

		if localPath != "" {
			if err := os.WriteFile(localPath, content, 0644); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to write local file: %v", err)), nil
			}
			return mcp.NewToolResultText(
				fmt.Sprintf("copied %d bytes from %s:%s to %s", len(content), pod, srcPath, localPath),
			), nil
		}

		return mcp.NewToolResultText(formatCopyFromResult(srcPath, content)), nil
	})
}

// extractTarFile reads the first file entry from a tar stream and returns its bytes.
// Returns an error if the file exceeds maxCopyBytes.
func extractTarFile(r *bytes.Buffer) ([]byte, error) {
	tr := tar.NewReader(r)
	_, err := tr.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to read tar header: %w", err)
	}

	limited := io.LimitReader(tr, int64(maxCopyBytes)+1)
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(limited); err != nil {
		return nil, fmt.Errorf("failed to read tar content: %w", err)
	}
	if buf.Len() > maxCopyBytes {
		return nil, fmt.Errorf("file exceeds maximum allowed size of %d MB", maxCopyBytes/(1024*1024))
	}
	return buf.Bytes(), nil
}

// formatCopyFromResult builds the tool response text.
func formatCopyFromResult(srcPath string, content []byte) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "path: %s\nsize: %d bytes\n", srcPath, len(content))

	if len(content) == 0 {
		sb.WriteString("encoding: text\n\n(empty file)")
		return sb.String()
	}

	if utf8.Valid(content) {
		sb.WriteString("encoding: text\n\n")
		sb.Write(content)
	} else {
		sb.WriteString("encoding: base64\n\n")
		sb.WriteString(base64.StdEncoding.EncodeToString(content))
	}
	return sb.String()
}
