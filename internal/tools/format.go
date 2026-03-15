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
func formatResourceList(items []unstructured.Unstructured) (string, error) {
	if len(items) == 0 {
		return "[]", nil
	}

	kind := items[0].GetKind()
	summaries := make([]map[string]interface{}, 0, len(items))

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
		return "", err
	}
	return string(out), nil
}

func baseFields(item unstructured.Unstructured) map[string]interface{} {
	m := map[string]interface{}{
		"name": item.GetName(),
		"age":  resourceAge(item),
	}
	if ns := item.GetNamespace(); ns != "" {
		m["namespace"] = ns
	}
	return m
}

func enrichPod(s map[string]interface{}, obj map[string]interface{}) {
	s["status"] = podStatus(obj)
	s["ready"] = podReady(obj)
	s["restarts"] = podRestarts(obj)
	s["node"] = getStrField(obj, "spec", "nodeName")
}

func enrichDeployment(s map[string]interface{}, obj map[string]interface{}) {
	replicas := getIntField(obj, "spec", "replicas")
	ready := getIntField(obj, "status", "readyReplicas")
	s["ready"] = fmt.Sprintf("%d/%d", ready, replicas)
	s["upToDate"] = getIntField(obj, "status", "updatedReplicas")
	s["available"] = getIntField(obj, "status", "availableReplicas")
}

func enrichStatefulSet(s map[string]interface{}, obj map[string]interface{}) {
	replicas := getIntField(obj, "spec", "replicas")
	ready := getIntField(obj, "status", "readyReplicas")
	s["ready"] = fmt.Sprintf("%d/%d", ready, replicas)
}

func enrichDaemonSet(s map[string]interface{}, obj map[string]interface{}) {
	s["desired"] = getIntField(obj, "status", "desiredNumberScheduled")
	s["ready"] = getIntField(obj, "status", "numberReady")
	s["available"] = getIntField(obj, "status", "numberAvailable")
}

func enrichJob(s map[string]interface{}, obj map[string]interface{}) {
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

func enrichNode(s map[string]interface{}, item unstructured.Unstructured) {
	status := "NotReady"
	if conditionIsTrue(item.Object, "Ready") {
		status = "Ready"
	}
	s["status"] = status
	s["roles"] = nodeRoles(item.GetLabels())
}

func enrichService(s map[string]interface{}, obj map[string]interface{}) {
	s["type"] = getStrField(obj, "spec", "type")
	s["clusterIP"] = getStrField(obj, "spec", "clusterIP")
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
