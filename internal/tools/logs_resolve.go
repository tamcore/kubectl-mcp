package tools

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// supportedResourceKinds lists the resource kinds that can be resolved to a label selector.
var supportedResourceKinds = map[string]bool{
	"deployment":  true,
	"job":         true,
	"statefulset": true,
	"replicaset":  true,
	"daemonset":   true,
	"cronjob":     true,
}

// parseResourceRef parses a "kind/name" string into kind and name.
// Returns an error if the format is invalid.
func parseResourceRef(resource string) (string, string, error) {
	parts := strings.SplitN(resource, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid resource format %q: expected 'kind/name' (e.g. 'deployment/nginx')", resource)
	}
	return parts[0], parts[1], nil
}

// resolveResourceToLabelSelector resolves a resource reference to a label selector
// by fetching the resource and extracting spec.selector.matchLabels.
func resolveResourceToLabelSelector(ctx context.Context, cc *kube.ContextClient, namespace, resource string) (string, error) {
	kind, name, err := parseResourceRef(resource)
	if err != nil {
		return "", err
	}

	lowerKind := strings.ToLower(kind)
	// Resolve short names.
	if fullKind, ok := resolveShortName(lowerKind); ok {
		lowerKind = strings.ToLower(fullKind)
		kind = fullKind
	}

	if !supportedResourceKinds[lowerKind] {
		return "", fmt.Errorf("resource kind %q is not supported for log resolution (supported: CronJob, DaemonSet, Deployment, Job, ReplicaSet, StatefulSet)", kind)
	}

	gvr, err := resolveGVR(cc, kind, "")
	if err != nil {
		return "", fmt.Errorf("failed to resolve resource kind %q: %w", kind, err)
	}

	obj, err := cc.Dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get %s/%s: %w", kind, name, err)
	}

	// CronJobs don't have spec.selector; resolve via their most recent Job.
	if lowerKind == "cronjob" {
		return resolveCronJobToLabelSelector(ctx, cc, namespace, obj)
	}

	// All other supported kinds have spec.selector.matchLabels.
	return extractMatchLabels(kind, name, obj)
}

// resolveCronJobToLabelSelector finds the most recent Job owned by a CronJob
// and returns that Job's spec.selector.matchLabels as a label selector string.
func resolveCronJobToLabelSelector(ctx context.Context, cc *kube.ContextClient, namespace string, cronJob *unstructured.Unstructured) (string, error) {
	cronJobName := cronJob.GetName()
	cronJobUID := string(cronJob.GetUID())

	jobGVR := schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
	jobList, err := cc.Dynamic.Resource(jobGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list jobs for CronJob/%s: %w", cronJobName, err)
	}

	// Find the most recent Job owned by this CronJob.
	var newest *unstructured.Unstructured
	for i := range jobList.Items {
		job := &jobList.Items[i]
		for _, ref := range job.GetOwnerReferences() {
			if ref.UID == cronJob.GetUID() {
				if newest == nil || job.GetCreationTimestamp().After(newest.GetCreationTimestamp().Time) {
					newest = job
				}
			}
		}
	}

	if newest == nil {
		return "", fmt.Errorf("no jobs found owned by CronJob/%s (uid %s)", cronJobName, cronJobUID)
	}

	return extractMatchLabels("Job", newest.GetName(), newest)
}

// extractMatchLabels extracts spec.selector.matchLabels from an unstructured
// resource and returns them as a comma-separated label selector string.
func extractMatchLabels(kind, name string, obj *unstructured.Unstructured) (string, error) {
	spec, ok := obj.Object["spec"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("%s/%s has no spec", kind, name)
	}

	selector, ok := spec["selector"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("%s/%s has no spec.selector", kind, name)
	}

	matchLabels, ok := selector["matchLabels"].(map[string]any)
	if !ok || len(matchLabels) == 0 {
		return "", fmt.Errorf("%s/%s has no spec.selector.matchLabels", kind, name)
	}

	// Build label selector string.
	var parts []string
	for k, v := range matchLabels {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}

	return strings.Join(parts, ","), nil
}
