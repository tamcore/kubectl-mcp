package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerNodeStats(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("node_stats",
		mcp.WithDescription("Get node-level resource usage summary (CPU, memory) from the kubelet stats/summary API"),
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

		absPath := "/api/v1/nodes/" + node + "/proxy/stats/summary"
		stream, err := cc.Clientset.CoreV1().RESTClient().Get().AbsPath(absPath).Stream(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get stats from node %q: %v", node, err)), nil
		}
		defer func() { _ = stream.Close() }()

		data, err := io.ReadAll(stream)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to read stats response: %v", err)), nil
		}

		if len(data) == 0 {
			return mcp.NewToolResultText("(no stats data)"), nil
		}

		summary, err := formatNodeStats(node, data)
		if err != nil {
			// If we can't parse the JSON, return raw data.
			return mcp.NewToolResultText(string(data)), nil
		}
		return mcp.NewToolResultText(summary), nil
	})
}

// nodeStatsSummary holds the parsed kubelet stats/summary response (partial).
type nodeStatsSummary struct {
	Node struct {
		NodeName string `json:"nodeName"`
		CPU      struct {
			UsageNanoCores       *int64 `json:"usageNanoCores"`
			UsageCoreNanoSeconds *int64 `json:"usageCoreNanoSeconds"`
		} `json:"cpu"`
		Memory struct {
			AvailableBytes  *int64 `json:"availableBytes"`
			UsageBytes      *int64 `json:"usageBytes"`
			WorkingSetBytes *int64 `json:"workingSetBytes"`
			RSSBytes        *int64 `json:"rssBytes"`
		} `json:"memory"`
		Fs struct {
			AvailableBytes *int64 `json:"availableBytes"`
			CapacityBytes  *int64 `json:"capacityBytes"`
			UsedBytes      *int64 `json:"usedBytes"`
		} `json:"fs"`
	} `json:"node"`
}

// formatNodeStats parses the kubelet stats/summary JSON and formats a human-readable summary.
func formatNodeStats(node string, data []byte) (string, error) {
	var stats nodeStatsSummary
	if err := json.Unmarshal(data, &stats); err != nil {
		return "", fmt.Errorf("failed to parse stats JSON: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Node: %s\n\n", node)

	// CPU
	fmt.Fprintf(&sb, "CPU:\n")
	if stats.Node.CPU.UsageNanoCores != nil {
		milliCores := float64(*stats.Node.CPU.UsageNanoCores) / 1e6
		fmt.Fprintf(&sb, "  Usage: %.1fm\n", milliCores)
	} else {
		fmt.Fprintf(&sb, "  Usage: <unavailable>\n")
	}

	// Memory
	fmt.Fprintf(&sb, "\nMemory:\n")
	if stats.Node.Memory.WorkingSetBytes != nil {
		fmt.Fprintf(&sb, "  Working Set: %s\n", formatBytes(*stats.Node.Memory.WorkingSetBytes))
	}
	if stats.Node.Memory.UsageBytes != nil {
		fmt.Fprintf(&sb, "  Usage:       %s\n", formatBytes(*stats.Node.Memory.UsageBytes))
	}
	if stats.Node.Memory.AvailableBytes != nil {
		fmt.Fprintf(&sb, "  Available:   %s\n", formatBytes(*stats.Node.Memory.AvailableBytes))
	}
	if stats.Node.Memory.RSSBytes != nil {
		fmt.Fprintf(&sb, "  RSS:         %s\n", formatBytes(*stats.Node.Memory.RSSBytes))
	}

	// Filesystem
	if stats.Node.Fs.CapacityBytes != nil {
		fmt.Fprintf(&sb, "\nFilesystem:\n")
		fmt.Fprintf(&sb, "  Capacity:  %s\n", formatBytes(*stats.Node.Fs.CapacityBytes))
		if stats.Node.Fs.UsedBytes != nil {
			fmt.Fprintf(&sb, "  Used:      %s\n", formatBytes(*stats.Node.Fs.UsedBytes))
		}
		if stats.Node.Fs.AvailableBytes != nil {
			fmt.Fprintf(&sb, "  Available: %s\n", formatBytes(*stats.Node.Fs.AvailableBytes))
		}
	}

	return sb.String(), nil
}

// formatBytes formats a byte count into a human-readable string.
func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1fGi", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1fMi", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1fKi", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
