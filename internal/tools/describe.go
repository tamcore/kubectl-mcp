package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/duration"
	"sigs.k8s.io/yaml"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerDescribeResource(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("describe_resource",
		mcp.WithRawOutputSchema(rawK8sObjectSchema),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Describe a Kubernetes resource with detailed information including conditions and events"),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("apiVersion",
			mcp.Description("API version (e.g. v1, apps/v1). If omitted, the server will try to discover it."),
		),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("Resource kind (e.g. Pod, Deployment, Service)"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Resource name"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace (required for namespaced resources)"),
		),
		mcp.WithString("include_annotations",
			mcp.Description("Comma-separated glob patterns for annotation keys to include (e.g. 'app.kubernetes.io/*'). If set, only matching annotations are returned."),
		),
		mcp.WithString("exclude_annotations",
			mcp.Description("Comma-separated glob patterns for annotation keys to exclude (e.g. 'kubectl.kubernetes.io/*'). "+
				"kubectl.kubernetes.io/last-applied-configuration is always excluded."),
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

		kind, _ := req.RequireString("kind")
		name, _ := req.RequireString("name")
		namespace := req.GetString("namespace", "")
		apiVersion := req.GetString("apiVersion", "")

		gvr, err := resolveGVR(cc, kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var obj *unstructured.Unstructured
		if namespace != "" {
			obj, err = cc.Dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		} else {
			obj, err = cc.Dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get %s/%s: %v", kind, name, err)), nil
		}

		if !cfg.AllowSecrets {
			kube.RedactSecrets(obj)
		}
		filterObjAnnotations(obj, req)

		// Strip noisy metadata before building output.
		cleaned := StripNoisyMetadata(obj.Object)

		var sb strings.Builder

		// Header
		fmt.Fprintf(&sb, "Name:         %s\n", obj.GetName())
		fmt.Fprintf(&sb, "Namespace:    %s\n", obj.GetNamespace())
		fmt.Fprintf(&sb, "Kind:         %s\n", obj.GetKind())
		fmt.Fprintf(&sb, "API Version:  %s\n", obj.GetAPIVersion())

		// Labels
		if labels := obj.GetLabels(); len(labels) > 0 {
			fmt.Fprintf(&sb, "Labels:\n")
			for k, v := range labels {
				fmt.Fprintf(&sb, "  %s=%s\n", k, v)
			}
		}

		// Annotations
		if annotations := obj.GetAnnotations(); len(annotations) > 0 {
			fmt.Fprintf(&sb, "Annotations:\n")
			for k, v := range annotations {
				fmt.Fprintf(&sb, "  %s=%s\n", k, v)
			}
		}

		// Age
		if ts := obj.GetCreationTimestamp(); !ts.IsZero() {
			fmt.Fprintf(&sb, "Age:          %s\n", duration.HumanDuration(metav1.Now().Sub(ts.Time)))
		}

		// Conditions (if present)
		conditions, found, _ := unstructuredNestedSlice(cleaned, "status", "conditions")
		if found && len(conditions) > 0 {
			fmt.Fprintf(&sb, "\nConditions:\n")
			fmt.Fprintf(&sb, "  %-25s %-10s %-25s %s\n", "TYPE", "STATUS", "REASON", "MESSAGE")
			for _, c := range conditions {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				fmt.Fprintf(&sb, "  %-25s %-10s %-25s %s\n",
					mapStr(cm, "type"),
					mapStr(cm, "status"),
					mapStr(cm, "reason"),
					mapStr(cm, "message"),
				)
			}
		}

		// Spec summary (YAML)
		if spec, ok := cleaned["spec"]; ok {
			specYAML, err := yaml.Marshal(spec)
			if err == nil {
				fmt.Fprintf(&sb, "\nSpec:\n")
				for line := range strings.SplitSeq(string(specYAML), "\n") {
					if line != "" {
						fmt.Fprintf(&sb, "  %s\n", line)
					}
				}
			}
		}

		// Fetch related events
		events, err := fetchEvents(ctx, cc, namespace, kind, name)
		if err == nil && events != "" {
			fmt.Fprintf(&sb, "\nEvents:\n%s", events)
		}

		return mcp.NewToolResultStructured(cleaned, sb.String()), nil
	})
}

func unstructuredNestedSlice(obj map[string]any, fields ...string) ([]any, bool, error) {
	current := obj
	for i, field := range fields {
		val, ok := current[field]
		if !ok {
			return nil, false, nil
		}
		if i == len(fields)-1 {
			slice, ok := val.([]any)
			return slice, ok, nil
		}
		next, ok := val.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		current = next
	}
	return nil, false, nil
}

func mapStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// fetchEvents retrieves events related to the given resource.
func fetchEvents(ctx context.Context, cc *kube.ContextClient, namespace, kind, name string) (string, error) {
	fieldSelector := fields.Set{
		"involvedObject.kind": kind,
		"involvedObject.name": name,
	}.String()
	events, err := cc.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return "", err
	}
	if len(events.Items) == 0 {
		return "  <none>\n", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "  %-8s %-10s %-25s %s\n", "TYPE", "REASON", "AGE", "MESSAGE")
	for _, e := range events.Items {
		age := "<unknown>"
		if !e.LastTimestamp.IsZero() {
			age = duration.HumanDuration(metav1.Now().Sub(e.LastTimestamp.Time))
		}
		fmt.Fprintf(&sb, "  %-8s %-10s %-25s %s\n", e.Type, e.Reason, age, e.Message)
	}
	return sb.String(), nil
}
