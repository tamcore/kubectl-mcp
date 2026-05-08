package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// makeUnstructured is a test helper that builds an Unstructured object.
func makeUnstructured(kind, name, namespace string, extra map[string]interface{}) unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":              name,
			"creationTimestamp": metav1.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	if namespace != "" {
		obj["metadata"].(map[string]interface{})["namespace"] = namespace
	}
	for k, v := range extra {
		obj[k] = v
	}
	return unstructured.Unstructured{Object: obj}
}

func TestFormatResourceList_Empty(t *testing.T) {
	got, _, err := formatResourceList(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "[]" {
		t.Errorf("expected [], got %s", got)
	}
}

func TestFormatResourceList_Pod(t *testing.T) {
	items := []unstructured.Unstructured{
		makeUnstructured("Pod", "nginx", "default", map[string]interface{}{
			"status": map[string]interface{}{
				"phase": "Running",
				"containerStatuses": []interface{}{
					map[string]interface{}{
						"ready":        true,
						"restartCount": int64(0),
						"state":        map[string]interface{}{},
					},
				},
			},
			"spec": map[string]interface{}{
				"nodeName": "node-1",
			},
		}),
	}
	got, _, err := formatResourceList(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 item, got %d", len(parsed))
	}
	if parsed[0]["status"] != "Running" {
		t.Errorf("expected status Running, got %v", parsed[0]["status"])
	}
	if parsed[0]["ready"] != "1/1" {
		t.Errorf("expected ready 1/1, got %v", parsed[0]["ready"])
	}
	if parsed[0]["node"] != "node-1" {
		t.Errorf("expected node node-1, got %v", parsed[0]["node"])
	}
}

func TestFormatResourceList_Deployment(t *testing.T) {
	items := []unstructured.Unstructured{
		makeUnstructured("Deployment", "web", "default", map[string]interface{}{
			"spec":   map[string]interface{}{"replicas": int64(3)},
			"status": map[string]interface{}{"readyReplicas": int64(3), "updatedReplicas": int64(3), "availableReplicas": int64(3)},
		}),
	}
	got, _, err := formatResourceList(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed[0]["ready"] != "3/3" {
		t.Errorf("expected ready 3/3, got %v", parsed[0]["ready"])
	}
}

func TestFormatResourceList_StatefulSet(t *testing.T) {
	items := []unstructured.Unstructured{
		makeUnstructured("StatefulSet", "db", "default", map[string]interface{}{
			"spec":   map[string]interface{}{"replicas": int64(3)},
			"status": map[string]interface{}{"readyReplicas": int64(2)},
		}),
	}
	got, _, err := formatResourceList(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed[0]["ready"] != "2/3" {
		t.Errorf("expected ready 2/3, got %v", parsed[0]["ready"])
	}
}

func TestFormatResourceList_DaemonSet(t *testing.T) {
	items := []unstructured.Unstructured{
		makeUnstructured("DaemonSet", "agent", "kube-system", map[string]interface{}{
			"status": map[string]interface{}{
				"desiredNumberScheduled": int64(5),
				"numberReady":            int64(5),
				"numberAvailable":        int64(5),
			},
		}),
	}
	got, _, err := formatResourceList(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed[0]["desired"] != float64(5) {
		t.Errorf("expected desired 5, got %v", parsed[0]["desired"])
	}
}

func TestFormatResourceList_Job(t *testing.T) {
	tests := []struct {
		name       string
		conditions []interface{}
		wantStatus string
	}{
		{
			name:       "running",
			conditions: nil,
			wantStatus: "Running",
		},
		{
			name: "complete",
			conditions: []interface{}{
				map[string]interface{}{"type": "Complete", "status": "True"},
			},
			wantStatus: "Complete",
		},
		{
			name: "failed",
			conditions: []interface{}{
				map[string]interface{}{"type": "Failed", "status": "True"},
			},
			wantStatus: "Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := map[string]interface{}{"succeeded": int64(1)}
			if tt.conditions != nil {
				status["conditions"] = tt.conditions
			}
			items := []unstructured.Unstructured{
				makeUnstructured("Job", "batch-job", "default", map[string]interface{}{
					"spec":   map[string]interface{}{"completions": int64(1)},
					"status": status,
				}),
			}
			got, _, err := formatResourceList(items)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var parsed []map[string]interface{}
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Fatal(err)
			}
			if parsed[0]["status"] != tt.wantStatus {
				t.Errorf("expected status %q, got %v", tt.wantStatus, parsed[0]["status"])
			}
		})
	}
}

func TestFormatResourceList_Node(t *testing.T) {
	tests := []struct {
		name       string
		labels     map[string]interface{}
		conditions []interface{}
		wantStatus string
		wantRoles  string
	}{
		{
			name:   "ready with roles",
			labels: map[string]interface{}{"node-role.kubernetes.io/control-plane": ""},
			conditions: []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
			wantStatus: "Ready",
			wantRoles:  "control-plane",
		},
		{
			name:       "not ready no roles",
			labels:     map[string]interface{}{},
			conditions: []interface{}{},
			wantStatus: "NotReady",
			wantRoles:  "<none>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Node",
				"metadata": map[string]interface{}{
					"name":              "node-1",
					"labels":            tt.labels,
					"creationTimestamp": metav1.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339),
				},
				"status": map[string]interface{}{"conditions": tt.conditions},
			}
			items := []unstructured.Unstructured{{Object: obj}}
			got, _, err := formatResourceList(items)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var parsed []map[string]interface{}
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Fatal(err)
			}
			if parsed[0]["status"] != tt.wantStatus {
				t.Errorf("expected status %q, got %v", tt.wantStatus, parsed[0]["status"])
			}
			if parsed[0]["roles"] != tt.wantRoles {
				t.Errorf("expected roles %q, got %v", tt.wantRoles, parsed[0]["roles"])
			}
		})
	}
}

func TestFormatResourceList_Service(t *testing.T) {
	tests := []struct {
		name    string
		svcType string
		ip      string
	}{
		{"ClusterIP", "ClusterIP", "10.0.0.1"},
		{"LoadBalancer", "LoadBalancer", "10.0.0.2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []unstructured.Unstructured{
				makeUnstructured("Service", "svc", "default", map[string]interface{}{
					"spec": map[string]interface{}{"type": tt.svcType, "clusterIP": tt.ip},
				}),
			}
			got, _, err := formatResourceList(items)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var parsed []map[string]interface{}
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Fatal(err)
			}
			if parsed[0]["type"] != tt.svcType {
				t.Errorf("expected type %q, got %v", tt.svcType, parsed[0]["type"])
			}
			if parsed[0]["clusterIP"] != tt.ip {
				t.Errorf("expected clusterIP %q, got %v", tt.ip, parsed[0]["clusterIP"])
			}
		})
	}
}

func TestFormatResourceList_UnknownKind(t *testing.T) {
	items := []unstructured.Unstructured{
		makeUnstructured("ConfigMap", "cm", "default", nil),
	}
	got, _, err := formatResourceList(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed[0]["name"] != "cm" {
		t.Errorf("expected name cm, got %v", parsed[0]["name"])
	}
	// Should only have base fields, no extra enrichment keys.
	if _, ok := parsed[0]["status"]; ok {
		t.Errorf("unexpected 'status' key for unknown kind")
	}
}

func TestBaseFields(t *testing.T) {
	t.Run("with namespace", func(t *testing.T) {
		u := makeUnstructured("Pod", "p", "ns1", nil)
		m := baseFields(u)
		if m["namespace"] != "ns1" {
			t.Errorf("expected namespace ns1, got %v", m["namespace"])
		}
		if m["name"] != "p" {
			t.Errorf("expected name p, got %v", m["name"])
		}
	})

	t.Run("cluster scoped", func(t *testing.T) {
		u := makeUnstructured("Node", "n1", "", nil)
		m := baseFields(u)
		if _, ok := m["namespace"]; ok {
			t.Error("expected no namespace key for cluster-scoped resource")
		}
	})
}

func TestPodStatus(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]interface{}
		want string
	}{
		{
			name: "phase only",
			obj: map[string]interface{}{
				"status": map[string]interface{}{"phase": "Running"},
			},
			want: "Running",
		},
		{
			name: "CrashLoopBackOff overrides phase",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
					"containerStatuses": []interface{}{
						map[string]interface{}{
							"state": map[string]interface{}{
								"waiting": map[string]interface{}{"reason": "CrashLoopBackOff"},
							},
						},
					},
				},
			},
			want: "CrashLoopBackOff",
		},
		{
			name: "terminated reason overrides phase",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Failed",
					"containerStatuses": []interface{}{
						map[string]interface{}{
							"state": map[string]interface{}{
								"terminated": map[string]interface{}{"reason": "OOMKilled"},
							},
						},
					},
				},
			},
			want: "OOMKilled",
		},
		{
			name: "init container waiting",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase":             "Pending",
					"containerStatuses": []interface{}{},
					"initContainerStatuses": []interface{}{
						map[string]interface{}{
							"state": map[string]interface{}{
								"waiting": map[string]interface{}{"reason": "ImagePullBackOff"},
							},
						},
					},
				},
			},
			want: "Init:ImagePullBackOff",
		},
		{
			name: "non-map container status is skipped",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase":             "Running",
					"containerStatuses": []interface{}{"not-a-map"},
				},
			},
			want: "Running",
		},
		{
			name: "non-map init container status is skipped",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase":                 "Pending",
					"initContainerStatuses": []interface{}{"not-a-map"},
				},
			},
			want: "Pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := podStatus(tt.obj)
			if got != tt.want {
				t.Errorf("podStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPodReady(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]interface{}
		want string
	}{
		{
			name: "2 of 3 ready",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"containerStatuses": []interface{}{
						map[string]interface{}{"ready": true},
						map[string]interface{}{"ready": true},
						map[string]interface{}{"ready": false},
					},
				},
			},
			want: "2/3",
		},
		{
			name: "no containers",
			obj:  map[string]interface{}{},
			want: "0/0",
		},
		{
			name: "non-map entry skipped",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"containerStatuses": []interface{}{
						"not-a-map",
						map[string]interface{}{"ready": true},
					},
				},
			},
			want: "1/2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := podReady(tt.obj)
			if got != tt.want {
				t.Errorf("podReady() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPodRestarts(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]interface{}
		want string
	}{
		{
			name: "int64 restart counts",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"containerStatuses": []interface{}{
						map[string]interface{}{"restartCount": int64(3)},
						map[string]interface{}{"restartCount": int64(2)},
					},
				},
			},
			want: "5",
		},
		{
			name: "float64 restart counts",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"containerStatuses": []interface{}{
						map[string]interface{}{"restartCount": float64(7)},
					},
				},
			},
			want: "7",
		},
		{
			name: "no containers",
			obj:  map[string]interface{}{},
			want: "0",
		},
		{
			name: "non-map entry skipped",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"containerStatuses": []interface{}{
						"not-a-map",
					},
				},
			},
			want: "0",
		},
		{
			name: "unsupported type for restartCount",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"containerStatuses": []interface{}{
						map[string]interface{}{"restartCount": "not-a-number"},
					},
				},
			},
			want: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := podRestarts(tt.obj)
			if got != tt.want {
				t.Errorf("podRestarts() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResourceAge(t *testing.T) {
	t.Run("valid timestamp", func(t *testing.T) {
		u := makeUnstructured("Pod", "p", "ns", nil)
		got := resourceAge(u)
		if got == "<unknown>" || got == "" {
			t.Errorf("expected valid age, got %q", got)
		}
	})

	t.Run("zero timestamp", func(t *testing.T) {
		u := unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "x"},
		}}
		got := resourceAge(u)
		if got != "<unknown>" {
			t.Errorf("expected <unknown>, got %q", got)
		}
	})
}

func TestGetStrField(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"nodeName": "node-1",
		},
		"intVal": int64(42),
	}

	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"nested string", []string{"spec", "nodeName"}, "node-1"},
		{"missing key", []string{"spec", "missing"}, ""},
		{"missing intermediate", []string{"nope", "nodeName"}, ""},
		{"non-string value uses Sprintf", []string{"intVal"}, "42"},
		{"non-map intermediate value", []string{"intVal", "sub"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStrField(obj, tt.keys...)
			if got != tt.want {
				t.Errorf("getStrField() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetIntField(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"readyReplicas": int64(3),
			"floatVal":      float64(7),
			"strVal":        "nope",
		},
	}

	tests := []struct {
		name string
		keys []string
		want int64
	}{
		{"int64 value", []string{"status", "readyReplicas"}, 3},
		{"float64 value", []string{"status", "floatVal"}, 7},
		{"wrong type", []string{"status", "strVal"}, 0},
		{"missing key", []string{"status", "missing"}, 0},
		{"missing intermediate", []string{"nope", "x"}, 0},
		{"non-map intermediate value", []string{"status", "readyReplicas", "sub"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getIntField(obj, tt.keys...)
			if got != tt.want {
				t.Errorf("getIntField() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestConditionIsTrue(t *testing.T) {
	tests := []struct {
		name     string
		obj      map[string]interface{}
		condType string
		want     bool
	}{
		{
			name: "condition True",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Ready", "status": "True"},
					},
				},
			},
			condType: "Ready",
			want:     true,
		},
		{
			name: "condition False",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Ready", "status": "False"},
					},
				},
			},
			condType: "Ready",
			want:     false,
		},
		{
			name: "condition missing",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{},
				},
			},
			condType: "Ready",
			want:     false,
		},
		{
			name: "non-map condition entry skipped",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{"not-a-map"},
				},
			},
			condType: "Ready",
			want:     false,
		},
		{
			name:     "no conditions field",
			obj:      map[string]interface{}{},
			condType: "Ready",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := conditionIsTrue(tt.obj, tt.condType)
			if got != tt.want {
				t.Errorf("conditionIsTrue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNestedSlice(t *testing.T) {
	tests := []struct {
		name      string
		obj       map[string]interface{}
		keys      []string
		wantLen   int
		wantFound bool
	}{
		{
			name:      "valid path",
			obj:       map[string]interface{}{"a": map[string]interface{}{"b": []interface{}{1, 2}}},
			keys:      []string{"a", "b"},
			wantLen:   2,
			wantFound: true,
		},
		{
			name:      "missing field",
			obj:       map[string]interface{}{},
			keys:      []string{"a", "b"},
			wantLen:   0,
			wantFound: false,
		},
		{
			name:      "wrong type at leaf",
			obj:       map[string]interface{}{"a": map[string]interface{}{"b": "not-a-slice"}},
			keys:      []string{"a", "b"},
			wantLen:   0,
			wantFound: false,
		},
		{
			name:      "non-map intermediate",
			obj:       map[string]interface{}{"a": "string"},
			keys:      []string{"a", "b"},
			wantLen:   0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found, _ := nestedSlice(tt.obj, tt.keys...)
			if found != tt.wantFound {
				t.Errorf("nestedSlice() found = %v, want %v", found, tt.wantFound)
			}
			if len(got) != tt.wantLen {
				t.Errorf("nestedSlice() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestNodeRoles(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			name:   "single role",
			labels: map[string]string{"node-role.kubernetes.io/worker": ""},
			want:   "worker",
		},
		{
			name:   "no roles",
			labels: map[string]string{"app": "something"},
			want:   "<none>",
		},
		{
			name:   "nil labels",
			labels: nil,
			want:   "<none>",
		},
		{
			name:   "empty role suffix ignored",
			labels: map[string]string{"node-role.kubernetes.io/": ""},
			want:   "<none>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nodeRoles(tt.labels)
			if tt.name == "single role" {
				if got != tt.want {
					t.Errorf("nodeRoles() = %q, want %q", got, tt.want)
				}
			} else {
				if got != tt.want {
					t.Errorf("nodeRoles() = %q, want %q", got, tt.want)
				}
			}
		})
	}

	// Multiple roles: order is non-deterministic from map iteration.
	t.Run("multiple roles", func(t *testing.T) {
		labels := map[string]string{
			"node-role.kubernetes.io/control-plane": "",
			"node-role.kubernetes.io/worker":        "",
		}
		got := nodeRoles(labels)
		if !strings.Contains(got, "control-plane") || !strings.Contains(got, "worker") {
			t.Errorf("nodeRoles() = %q, expected both control-plane and worker", got)
		}
	})
}

func TestEnrichDeployment_PartiallyAvailable(t *testing.T) {
	obj := map[string]interface{}{
		"spec":   map[string]interface{}{"replicas": int64(3)},
		"status": map[string]interface{}{"readyReplicas": int64(1), "updatedReplicas": int64(2), "availableReplicas": int64(1)},
	}
	s := map[string]interface{}{}
	enrichDeployment(s, obj)
	if s["ready"] != "1/3" {
		t.Errorf("expected 1/3, got %v", s["ready"])
	}
	if s["upToDate"] != int64(2) {
		t.Errorf("expected upToDate 2, got %v", s["upToDate"])
	}
	if s["available"] != int64(1) {
		t.Errorf("expected available 1, got %v", s["available"])
	}
}

func TestEnrichStatefulSet(t *testing.T) {
	obj := map[string]interface{}{
		"spec":   map[string]interface{}{"replicas": int64(5)},
		"status": map[string]interface{}{"readyReplicas": int64(5)},
	}
	s := map[string]interface{}{}
	enrichStatefulSet(s, obj)
	if s["ready"] != "5/5" {
		t.Errorf("expected 5/5, got %v", s["ready"])
	}
}

func TestEnrichDaemonSet(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"desiredNumberScheduled": int64(3),
			"numberReady":            int64(3),
			"numberAvailable":        int64(3),
		},
	}
	s := map[string]interface{}{}
	enrichDaemonSet(s, obj)
	if s["desired"] != int64(3) {
		t.Errorf("expected desired 3, got %v", s["desired"])
	}
	if s["ready"] != int64(3) {
		t.Errorf("expected ready 3, got %v", s["ready"])
	}
	if s["available"] != int64(3) {
		t.Errorf("expected available 3, got %v", s["available"])
	}
}
