package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerNodeLogs(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("node_logs",
		mcp.WithDescription("Get logs from a Kubernetes node via the kubelet proxy"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("node",
			mcp.Required(),
			mcp.Description("Node name"),
		),
		mcp.WithString("logPath",
			mcp.Description("Log file path under /var/log on the node (e.g. 'syslog', 'journal'). Defaults to root listing."),
		),
		mcp.WithNumber("tail",
			mcp.Description("Number of lines from the end to return (passed as query parameter)"),
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

		node, _ := req.RequireString("node")
		logPath := req.GetString("logPath", "")
		tail := int64(req.GetFloat("tail", 0))

		// Validate logPath to prevent path traversal.
		if err := validateLogPath(logPath); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		absPath := "/api/v1/nodes/" + node + "/proxy/logs/"
		if logPath != "" {
			absPath += logPath
		}

		restReq := cc.Clientset.CoreV1().RESTClient().Get().AbsPath(absPath)
		if tail > 0 {
			restReq.Param("tailLines", fmt.Sprintf("%d", tail))
		}

		stream, err := restReq.Stream(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get logs from node %q: %v", node, err)), nil
		}
		defer func() { _ = stream.Close() }()

		data, err := io.ReadAll(stream)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to read node logs: %v", err)), nil
		}

		if len(data) == 0 {
			return mcp.NewToolResultText("(no logs)"), nil
		}

		// Detect HTML directory listings returned by the kubelet and convert
		// them to a helpful message listing available log paths.
		if isHTMLDirListing(data) {
			links := parseHTMLDirLinks(data)
			if len(links) > 0 {
				return mcp.NewToolResultText(formatDirListing(logPath, links)), nil
			}
		}

		return mcp.NewToolResultText(string(data)), nil
	})
}

// isHTMLDirListing returns true if the data looks like an HTML directory listing
// from the kubelet's /var/log/ proxy endpoint.
func isHTMLDirListing(data []byte) bool {
	lower := bytes.ToLower(data)
	return (bytes.Contains(lower, []byte("<!doctype html")) || bytes.Contains(lower, []byte("<pre>"))) &&
		bytes.Contains(lower, []byte("<a href="))
}

// hrefRe matches href="..." attributes in HTML anchor tags.
var hrefRe = regexp.MustCompile(`<a\s+href="([^"]+)"`)

// parseHTMLDirLinks extracts href values from an HTML directory listing.
func parseHTMLDirLinks(data []byte) []string {
	matches := hrefRe.FindAllSubmatch(data, -1)
	links := make([]string, 0, len(matches))
	for _, m := range matches {
		links = append(links, string(m[1]))
	}
	return links
}

// formatDirListing builds a human-readable message from a directory listing,
// prefixing each entry with the parent logPath so users can call back with the full path.
func formatDirListing(logPath string, links []string) string {
	var sb strings.Builder
	sb.WriteString("The requested path returned a directory listing instead of log content.\n\n")
	sb.WriteString("Available entries:\n")

	prefix := ""
	if logPath != "" {
		prefix = strings.TrimSuffix(logPath, "/") + "/"
	}

	for _, link := range links {
		sb.WriteString("  - ")
		sb.WriteString(prefix)
		sb.WriteString(link)
		sb.WriteString("\n")
	}

	sb.WriteString("\nUse one of these paths as the logPath parameter to view the log content.")
	return sb.String()
}

// validateLogPath checks for path traversal attempts in the log path.
func validateLogPath(p string) error {
	if p == "" {
		return nil
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("logPath must be a relative path — path traversal is not allowed")
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("logPath must not contain '..' — path traversal is not allowed")
	}
	return nil
}
