package tools

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

var (
	podMetricsGVR  = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
	nodeMetricsGVR = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}
)

// formatCPU converts a Kubernetes CPU quantity string to human-readable millicores.
func formatCPU(raw string) string {
	q := resource.MustParse(raw)
	return fmt.Sprintf("%dm", q.MilliValue())
}

// formatMemory converts a Kubernetes memory quantity string to human-readable MiB.
func formatMemory(raw string) string {
	q := resource.MustParse(raw)
	mib := q.Value() / (1024 * 1024)
	return fmt.Sprintf("%dMi", mib)
}

// formatPercent returns "N/A" when allocatable is zero, otherwise "X%".
func formatPercent(used, allocatable int64) string {
	if allocatable == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%d%%", used*100/allocatable)
}

// isMetricsNotAvailable checks if the error indicates the metrics.k8s.io API
// group is not registered (i.e. metrics-server is not installed). It avoids
// matching "not found" errors for specific objects (e.g. a pod name).
func isMetricsNotAvailable(err error) bool {
	msg := err.Error()
	// API group not registered or CRD/aggregated-API missing.
	if strings.Contains(msg, "could not find the requested resource") ||
		strings.Contains(msg, "the server could not find the requested resource") ||
		strings.Contains(msg, "no matches for kind") {
		return true
	}
	// NotFound on the resource type itself (not a specific object name).
	if errors.IsNotFound(err) {
		var statusErr *errors.StatusError
		if stderrors.As(err, &statusErr) {
			// When metrics-server is missing, the "name" in the status is
			// typically empty or describes the resource, not a user-supplied
			// object name.
			details := statusErr.Status().Details
			if details != nil && details.Group == "metrics.k8s.io" {
				return true
			}
		}
	}
	return false
}

const metricsNotAvailableMsg = "metrics-server is not available in this cluster (the metrics.k8s.io API was not found). Install metrics-server to use this tool."

// ---------------------------------------------------------------------------
// top_pods
// ---------------------------------------------------------------------------

func registerTopPods(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("top_pods",
		mcp.WithDescription("Get CPU and memory usage for pods (like kubectl top pods). Requires metrics-server."),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace (omit for all namespaces)"),
		),
		mcp.WithString("name",
			mcp.Description("Filter by pod name"),
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

		namespace := req.GetString("namespace", "")
		name := req.GetString("name", "")

		var metricsList []unstructured.Unstructured

		if name != "" {
			obj, err := cc.Dynamic.Resource(podMetricsGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				if isMetricsNotAvailable(err) {
					return mcp.NewToolResultError(metricsNotAvailableMsg), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get pod metrics: %v", err)), nil
			}
			metricsList = []unstructured.Unstructured{*obj}
		} else {
			list, err := cc.Dynamic.Resource(podMetricsGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				if isMetricsNotAvailable(err) {
					return mcp.NewToolResultError(metricsNotAvailableMsg), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to list pod metrics: %v", err)), nil
			}
			metricsList = list.Items
		}

		if len(metricsList) == 0 {
			return mcp.NewToolResultText("[]"), nil
		}

		type podUsage struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			CPU       string `json:"cpu"`
			Memory    string `json:"memory"`
		}

		items := make([]podUsage, 0, len(metricsList))
		for _, m := range metricsList {
			cpuMillis, memBytes := sumContainerUsage(m.Object)
			items = append(items, podUsage{
				Name:      m.GetName(),
				Namespace: m.GetNamespace(),
				CPU:       fmt.Sprintf("%dm", cpuMillis),
				Memory:    fmt.Sprintf("%dMi", memBytes/(1024*1024)),
			})
		}

		out, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	})
}

// sumContainerUsage sums CPU (millicores) and memory (bytes) across all
// containers in a PodMetrics unstructured object.
func sumContainerUsage(obj map[string]interface{}) (cpuMillis int64, memBytes int64) {
	containers, ok := obj["containers"].([]interface{})
	if !ok {
		return 0, 0
	}
	for _, c := range containers {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		usage, ok := cm["usage"].(map[string]interface{})
		if !ok {
			continue
		}
		if cpu, ok := usage["cpu"].(string); ok {
			q := resource.MustParse(cpu)
			cpuMillis += q.MilliValue()
		}
		if mem, ok := usage["memory"].(string); ok {
			q := resource.MustParse(mem)
			memBytes += q.Value()
		}
	}
	return cpuMillis, memBytes
}

// ---------------------------------------------------------------------------
// top_nodes
// ---------------------------------------------------------------------------

func registerTopNodes(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("top_nodes",
		mcp.WithDescription("Get CPU and memory usage for nodes (like kubectl top nodes). Requires metrics-server."),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("name",
			mcp.Description("Filter by node name"),
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

		name := req.GetString("name", "")

		// Fetch node metrics first — fail fast if metrics-server is missing.
		var metricsList []unstructured.Unstructured
		if name != "" {
			obj, err := cc.Dynamic.Resource(nodeMetricsGVR).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				if isMetricsNotAvailable(err) {
					return mcp.NewToolResultError(metricsNotAvailableMsg), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get node metrics: %v", err)), nil
			}
			metricsList = []unstructured.Unstructured{*obj}
		} else {
			list, err := cc.Dynamic.Resource(nodeMetricsGVR).List(ctx, metav1.ListOptions{})
			if err != nil {
				if isMetricsNotAvailable(err) {
					return mcp.NewToolResultError(metricsNotAvailableMsg), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to list node metrics: %v", err)), nil
			}
			metricsList = list.Items
		}

		if len(metricsList) == 0 {
			return mcp.NewToolResultText("[]"), nil
		}

		// Fetch node objects for allocatable resources.
		allocatable := make(map[string]nodeAllocatable)
		nodeList, err := cc.Dynamic.Resource(nodeGVR).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, n := range nodeList.Items {
				alloc := extractAllocatable(n.Object)
				allocatable[n.GetName()] = alloc
			}
		}

		type nodeUsage struct {
			Name              string `json:"name"`
			CPUUsed           string `json:"cpuUsed"`
			CPUAllocatable    string `json:"cpuAllocatable"`
			CPUPercent        string `json:"cpuPercent"`
			MemoryUsed        string `json:"memoryUsed"`
			MemoryAllocatable string `json:"memoryAllocatable"`
			MemoryPercent     string `json:"memoryPercent"`
		}

		items := make([]nodeUsage, 0, len(metricsList))
		for _, m := range metricsList {
			usage, ok := m.Object["usage"].(map[string]interface{})
			if !ok {
				continue
			}

			var cpuMillis, memBytes int64
			if cpu, ok := usage["cpu"].(string); ok {
				q := resource.MustParse(cpu)
				cpuMillis = q.MilliValue()
			}
			if mem, ok := usage["memory"].(string); ok {
				q := resource.MustParse(mem)
				memBytes = q.Value()
			}

			alloc := allocatable[m.GetName()]

			items = append(items, nodeUsage{
				Name:              m.GetName(),
				CPUUsed:           fmt.Sprintf("%dm", cpuMillis),
				CPUAllocatable:    fmt.Sprintf("%dm", alloc.cpuMillis),
				CPUPercent:        formatPercent(cpuMillis, alloc.cpuMillis),
				MemoryUsed:        fmt.Sprintf("%dMi", memBytes/(1024*1024)),
				MemoryAllocatable: fmt.Sprintf("%dMi", alloc.memBytes/(1024*1024)),
				MemoryPercent:     formatPercent(memBytes, alloc.memBytes),
			})
		}

		out, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	})
}

type nodeAllocatable struct {
	cpuMillis int64
	memBytes  int64
}

// extractAllocatable pulls .status.allocatable.{cpu,memory} from a Node object.
func extractAllocatable(obj map[string]interface{}) nodeAllocatable {
	status, ok := obj["status"].(map[string]interface{})
	if !ok {
		return nodeAllocatable{}
	}
	alloc, ok := status["allocatable"].(map[string]interface{})
	if !ok {
		return nodeAllocatable{}
	}

	var result nodeAllocatable
	if cpu, ok := alloc["cpu"].(string); ok {
		q := resource.MustParse(cpu)
		result.cpuMillis = q.MilliValue()
	}
	if mem, ok := alloc["memory"].(string); ok {
		q := resource.MustParse(mem)
		result.memBytes = q.Value()
	}
	return result
}
