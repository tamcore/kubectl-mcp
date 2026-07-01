package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/duration"
)

// formatResourceList returns a JSON array of resource summaries.
// Each resource kind gets tailored fields so the LLM can reason
// over them without text parsing.
func formatResourceList(items []unstructured.Unstructured) (string, []map[string]any, error) {
	if len(items) == 0 {
		return "[]", nil, nil
	}

	kind := items[0].GetKind()
	summaries := make([]map[string]any, 0, len(items))

	for _, item := range items {
		s := baseFields(item)
		switch kind {
		case "Pod":
			enrichPod(s, item.Object)
		case "Deployment":
			enrichDeployment(s, item.Object)
		case "StatefulSet":
			enrichStatefulSet(s, item.Object)
		case "DaemonSet":
			enrichDaemonSet(s, item.Object)
		case "Job":
			enrichJob(s, item.Object)
		case "Node":
			enrichNode(s, item)
		case "Service":
			enrichService(s, item.Object)
		}
		summaries = append(summaries, s)
	}

	out, err := json.MarshalIndent(summaries, "", "  ")
	if err != nil {
		return "", nil, err
	}
	return string(out), summaries, nil
}

func baseFields(item unstructured.Unstructured) map[string]any {
	m := map[string]any{
		"name": item.GetName(),
		"age":  resourceAge(item),
	}
	if ns := item.GetNamespace(); ns != "" {
		m["namespace"] = ns
	}
	return m
}

func enrichPod(s map[string]any, obj map[string]any) {
	s["status"] = podStatus(obj)
	s["ready"] = podReady(obj)
	s["restarts"] = podRestarts(obj)
	s["node"] = getStrField(obj, "spec", "nodeName")
}

func enrichDeployment(s map[string]any, obj map[string]any) {
	replicas := getIntField(obj, "spec", "replicas")
	ready := getIntField(obj, "status", "readyReplicas")
	s["ready"] = fmt.Sprintf("%d/%d", ready, replicas)
	s["upToDate"] = getIntField(obj, "status", "updatedReplicas")
	s["available"] = getIntField(obj, "status", "availableReplicas")
}

func enrichStatefulSet(s map[string]any, obj map[string]any) {
	replicas := getIntField(obj, "spec", "replicas")
	ready := getIntField(obj, "status", "readyReplicas")
	s["ready"] = fmt.Sprintf("%d/%d", ready, replicas)
}

func enrichDaemonSet(s map[string]any, obj map[string]any) {
	s["desired"] = getIntField(obj, "status", "desiredNumberScheduled")
	s["ready"] = getIntField(obj, "status", "numberReady")
	s["available"] = getIntField(obj, "status", "numberAvailable")
}

func enrichJob(s map[string]any, obj map[string]any) {
	succeeded := getIntField(obj, "status", "succeeded")
	completions := getIntField(obj, "spec", "completions")
	s["completions"] = fmt.Sprintf("%d/%d", succeeded, completions)
	status := "Running"
	if conditionIsTrue(obj, "Complete") {
		status = "Complete"
	} else if conditionIsTrue(obj, "Failed") {
		status = "Failed"
	}
	s["status"] = status
}

func enrichNode(s map[string]any, item unstructured.Unstructured) {
	status := "NotReady"
	if conditionIsTrue(item.Object, "Ready") {
		status = "Ready"
	}
	s["status"] = status
	s["roles"] = nodeRoles(item.GetLabels())
}

func enrichService(s map[string]any, obj map[string]any) {
	s["type"] = getStrField(obj, "spec", "type")
	s["clusterIP"] = getStrField(obj, "spec", "clusterIP")
}

// --- Pod helpers ---

func podStatus(obj map[string]any) string {
	phase := getStrField(obj, "status", "phase")

	// Check for container-level overrides (CrashLoopBackOff, etc.)
	containerStatuses, _, _ := nestedSlice(obj, "status", "containerStatuses")
	for _, cs := range containerStatuses {
		cm, ok := cs.(map[string]any)
		if !ok {
			continue
		}
		if reason := containerStateReason(cm, "waiting"); reason != "" {
			return reason
		}
		if reason := containerStateReason(cm, "terminated"); reason != "" {
			return reason
		}
	}

	// Check init containers too.
	initStatuses, _, _ := nestedSlice(obj, "status", "initContainerStatuses")
	for _, cs := range initStatuses {
		cm, ok := cs.(map[string]any)
		if !ok {
			continue
		}
		if reason := containerStateReason(cm, "waiting"); reason != "" {
			return "Init:" + reason
		}
	}

	return phase
}

// containerStateReason safely extracts .state.<stateKey>.reason from a single
// containerStatus map. It returns "" when any level is missing or not the
// expected type, avoiding panics on unexpected status shapes.
func containerStateReason(cm map[string]any, stateKey string) string {
	state, ok := cm["state"].(map[string]any)
	if !ok {
		return ""
	}
	sub, ok := state[stateKey].(map[string]any)
	if !ok {
		return ""
	}
	reason, _ := sub["reason"].(string)
	return reason
}

func podReady(obj map[string]any) string {
	containerStatuses, _, _ := nestedSlice(obj, "status", "containerStatuses")
	total := len(containerStatuses)
	ready := 0
	for _, cs := range containerStatuses {
		cm, ok := cs.(map[string]any)
		if !ok {
			continue
		}
		if r, ok := cm["ready"].(bool); ok && r {
			ready++
		}
	}
	return fmt.Sprintf("%d/%d", ready, total)
}

func podRestarts(obj map[string]any) string {
	containerStatuses, _, _ := nestedSlice(obj, "status", "containerStatuses")
	total := 0
	for _, cs := range containerStatuses {
		cm, ok := cs.(map[string]any)
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

func getStrField(obj map[string]any, keys ...string) string {
	current := any(obj)
	for _, k := range keys {
		m, ok := current.(map[string]any)
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

func getIntField(obj map[string]any, keys ...string) int64 {
	current := any(obj)
	for _, k := range keys {
		m, ok := current.(map[string]any)
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

func conditionIsTrue(obj map[string]any, condType string) bool {
	conditions, _, _ := nestedSlice(obj, "status", "conditions")
	for _, c := range conditions {
		cm, ok := c.(map[string]any)
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

func nestedSlice(obj map[string]any, keys ...string) ([]any, bool, error) {
	current := any(obj)
	for _, k := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		current, ok = m[k]
		if !ok {
			return nil, false, nil
		}
	}
	s, ok := current.([]any)
	return s, ok, nil
}

func nodeRoles(labels map[string]string) string {
	var roles []string
	for k := range labels {
		if after, ok := strings.CutPrefix(k, "node-role.kubernetes.io/"); ok {
			role := after
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

// formatTable renders a metav1.Table as a human-readable, column-aligned text
// table. Column headers come from ColumnDefinitions; row data from Cells.
func formatTable(table *metav1.Table) string {
	if table == nil || len(table.Rows) == 0 {
		return "(no data)"
	}

	// Build header names.
	headers := make([]string, 0, len(table.ColumnDefinitions))
	for _, col := range table.ColumnDefinitions {
		headers = append(headers, strings.ToUpper(col.Name))
	}

	// Compute column widths.
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	rows := make([][]string, 0, len(table.Rows))
	for _, row := range table.Rows {
		cells := make([]string, len(headers))
		for i := range headers {
			if i < len(row.Cells) {
				cells[i] = fmt.Sprintf("%v", row.Cells[i])
			}
			if len(cells[i]) > widths[i] {
				widths[i] = len(cells[i])
			}
		}
		rows = append(rows, cells)
	}

	// Render.
	var sb strings.Builder
	for i, h := range headers {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(padRight(h, widths[i]))
	}
	sb.WriteString("\n")

	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				sb.WriteString("  ")
			}
			sb.WriteString(padRight(cell, widths[i]))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// padRight pads s with spaces on the right to the given width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
