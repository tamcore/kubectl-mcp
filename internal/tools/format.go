package tools

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/duration"
)

// formatResourceTable writes a kubectl-like table for the given items.
// It detects the resource kind and adds appropriate status columns.
func formatResourceTable(sb *strings.Builder, items []unstructured.Unstructured) {
	if len(items) == 0 {
		return
	}

	kind := items[0].GetKind()
	switch kind {
	case "Pod":
		formatPodTable(sb, items)
	case "Deployment":
		formatDeploymentTable(sb, items)
	case "StatefulSet":
		formatStatefulSetTable(sb, items)
	case "DaemonSet":
		formatDaemonSetTable(sb, items)
	case "Job":
		formatJobTable(sb, items)
	case "Node":
		formatNodeTable(sb, items)
	case "Service":
		formatServiceTable(sb, items)
	default:
		formatGenericTable(sb, items)
	}
}

func formatPodTable(sb *strings.Builder, items []unstructured.Unstructured) {
	fmt.Fprintf(sb, "%-50s %-20s %-10s %-10s %-10s %s\n",
		"NAME", "NAMESPACE", "STATUS", "READY", "RESTARTS", "AGE")
	for _, item := range items {
		status := podStatus(item.Object)
		ready := podReady(item.Object)
		restarts := podRestarts(item.Object)
		fmt.Fprintf(sb, "%-50s %-20s %-10s %-10s %-10s %s\n",
			item.GetName(), item.GetNamespace(), status, ready, restarts, resourceAge(item))
	}
}

func formatDeploymentTable(sb *strings.Builder, items []unstructured.Unstructured) {
	fmt.Fprintf(sb, "%-50s %-20s %-12s %-12s %-12s %s\n",
		"NAME", "NAMESPACE", "READY", "UP-TO-DATE", "AVAILABLE", "AGE")
	for _, item := range items {
		replicas := getIntField(item.Object, "spec", "replicas")
		ready := getIntField(item.Object, "status", "readyReplicas")
		upToDate := getIntField(item.Object, "status", "updatedReplicas")
		available := getIntField(item.Object, "status", "availableReplicas")
		fmt.Fprintf(sb, "%-50s %-20s %-12s %-12s %-12s %s\n",
			item.GetName(), item.GetNamespace(),
			fmt.Sprintf("%d/%d", ready, replicas),
			fmt.Sprintf("%d", upToDate),
			fmt.Sprintf("%d", available),
			resourceAge(item))
	}
}

func formatStatefulSetTable(sb *strings.Builder, items []unstructured.Unstructured) {
	fmt.Fprintf(sb, "%-50s %-20s %-12s %s\n",
		"NAME", "NAMESPACE", "READY", "AGE")
	for _, item := range items {
		replicas := getIntField(item.Object, "spec", "replicas")
		ready := getIntField(item.Object, "status", "readyReplicas")
		fmt.Fprintf(sb, "%-50s %-20s %-12s %s\n",
			item.GetName(), item.GetNamespace(),
			fmt.Sprintf("%d/%d", ready, replicas),
			resourceAge(item))
	}
}

func formatDaemonSetTable(sb *strings.Builder, items []unstructured.Unstructured) {
	fmt.Fprintf(sb, "%-50s %-20s %-10s %-10s %-10s %s\n",
		"NAME", "NAMESPACE", "DESIRED", "READY", "AVAILABLE", "AGE")
	for _, item := range items {
		desired := getIntField(item.Object, "status", "desiredNumberScheduled")
		ready := getIntField(item.Object, "status", "numberReady")
		available := getIntField(item.Object, "status", "numberAvailable")
		fmt.Fprintf(sb, "%-50s %-20s %-10d %-10d %-10d %s\n",
			item.GetName(), item.GetNamespace(),
			desired, ready, available, resourceAge(item))
	}
}

func formatJobTable(sb *strings.Builder, items []unstructured.Unstructured) {
	fmt.Fprintf(sb, "%-50s %-20s %-12s %-12s %s\n",
		"NAME", "NAMESPACE", "COMPLETIONS", "STATUS", "AGE")
	for _, item := range items {
		succeeded := getIntField(item.Object, "status", "succeeded")
		completions := getIntField(item.Object, "spec", "completions")
		status := "Running"
		if conditionIsTrue(item.Object, "Complete") {
			status = "Complete"
		} else if conditionIsTrue(item.Object, "Failed") {
			status = "Failed"
		}
		fmt.Fprintf(sb, "%-50s %-20s %-12s %-12s %s\n",
			item.GetName(), item.GetNamespace(),
			fmt.Sprintf("%d/%d", succeeded, completions),
			status, resourceAge(item))
	}
}

func formatNodeTable(sb *strings.Builder, items []unstructured.Unstructured) {
	fmt.Fprintf(sb, "%-50s %-10s %-30s %s\n",
		"NAME", "STATUS", "ROLES", "AGE")
	for _, item := range items {
		status := "NotReady"
		if conditionIsTrue(item.Object, "Ready") {
			status = "Ready"
		}
		roles := nodeRoles(item.GetLabels())
		fmt.Fprintf(sb, "%-50s %-10s %-30s %s\n",
			item.GetName(), status, roles, resourceAge(item))
	}
}

func formatServiceTable(sb *strings.Builder, items []unstructured.Unstructured) {
	fmt.Fprintf(sb, "%-50s %-20s %-12s %-25s %s\n",
		"NAME", "NAMESPACE", "TYPE", "CLUSTER-IP", "AGE")
	for _, item := range items {
		svcType := getStrField(item.Object, "spec", "type")
		clusterIP := getStrField(item.Object, "spec", "clusterIP")
		fmt.Fprintf(sb, "%-50s %-20s %-12s %-25s %s\n",
			item.GetName(), item.GetNamespace(), svcType, clusterIP, resourceAge(item))
	}
}

func formatGenericTable(sb *strings.Builder, items []unstructured.Unstructured) {
	fmt.Fprintf(sb, "%-50s %-20s %s\n", "NAME", "NAMESPACE", "AGE")
	for _, item := range items {
		fmt.Fprintf(sb, "%-50s %-20s %s\n",
			item.GetName(), item.GetNamespace(), resourceAge(item))
	}
}

// --- Pod helpers ---

func podStatus(obj map[string]interface{}) string {
	phase := getStrField(obj, "status", "phase")

	// Check for container-level overrides (CrashLoopBackOff, etc.)
	containerStatuses, _, _ := nestedSlice(obj, "status", "containerStatuses")
	for _, cs := range containerStatuses {
		cm, ok := cs.(map[string]interface{})
		if !ok {
			continue
		}
		if waiting, ok := cm["state"].(map[string]interface{})["waiting"].(map[string]interface{}); ok {
			if reason, ok := waiting["reason"].(string); ok && reason != "" {
				return reason
			}
		}
		if terminated, ok := cm["state"].(map[string]interface{})["terminated"].(map[string]interface{}); ok {
			if reason, ok := terminated["reason"].(string); ok && reason != "" {
				return reason
			}
		}
	}

	// Check init containers too.
	initStatuses, _, _ := nestedSlice(obj, "status", "initContainerStatuses")
	for _, cs := range initStatuses {
		cm, ok := cs.(map[string]interface{})
		if !ok {
			continue
		}
		if waiting, ok := cm["state"].(map[string]interface{})["waiting"].(map[string]interface{}); ok {
			if reason, ok := waiting["reason"].(string); ok && reason != "" {
				return "Init:" + reason
			}
		}
	}

	return phase
}

func podReady(obj map[string]interface{}) string {
	containerStatuses, _, _ := nestedSlice(obj, "status", "containerStatuses")
	total := len(containerStatuses)
	ready := 0
	for _, cs := range containerStatuses {
		cm, ok := cs.(map[string]interface{})
		if !ok {
			continue
		}
		if r, ok := cm["ready"].(bool); ok && r {
			ready++
		}
	}
	return fmt.Sprintf("%d/%d", ready, total)
}

func podRestarts(obj map[string]interface{}) string {
	containerStatuses, _, _ := nestedSlice(obj, "status", "containerStatuses")
	total := 0
	for _, cs := range containerStatuses {
		cm, ok := cs.(map[string]interface{})
		if !ok {
			continue
		}
		if rc, ok := cm["restartCount"].(int64); ok {
			total += int(rc)
		} else if rc, ok := cm["restartCount"].(float64); ok {
			total += int(rc)
		}
	}
	return fmt.Sprintf("%d", total)
}

// --- Generic helpers ---

func resourceAge(item unstructured.Unstructured) string {
	ts := item.GetCreationTimestamp()
	if ts.IsZero() {
		return "<unknown>"
	}
	return duration.HumanDuration(metav1.Now().Sub(ts.Time))
}

func getStrField(obj map[string]interface{}, keys ...string) string {
	current := interface{}(obj)
	for _, k := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current, ok = m[k]
		if !ok {
			return ""
		}
	}
	s, ok := current.(string)
	if !ok {
		return fmt.Sprintf("%v", current)
	}
	return s
}

func getIntField(obj map[string]interface{}, keys ...string) int64 {
	current := interface{}(obj)
	for _, k := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return 0
		}
		current, ok = m[k]
		if !ok {
			return 0
		}
	}
	switch v := current.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func conditionIsTrue(obj map[string]interface{}, condType string) bool {
	conditions, _, _ := nestedSlice(obj, "status", "conditions")
	for _, c := range conditions {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := cm["type"].(string); t == condType {
			if s, _ := cm["status"].(string); s == "True" {
				return true
			}
		}
	}
	return false
}

func nestedSlice(obj map[string]interface{}, keys ...string) ([]interface{}, bool, error) {
	current := interface{}(obj)
	for _, k := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false, nil
		}
		current, ok = m[k]
		if !ok {
			return nil, false, nil
		}
	}
	s, ok := current.([]interface{})
	return s, ok, nil
}

func nodeRoles(labels map[string]string) string {
	var roles []string
	for k := range labels {
		if strings.HasPrefix(k, "node-role.kubernetes.io/") {
			role := strings.TrimPrefix(k, "node-role.kubernetes.io/")
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	if len(roles) == 0 {
		return "<none>"
	}
	return strings.Join(roles, ",")
}
