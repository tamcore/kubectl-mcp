package tools

import "testing"

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "b", 1},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"Deployment", "Deploymnt", 1},
		{"Deployment", "Deploment", 1},
		{"Deployment", "deployment", 1},     // case differs in first char
		{"StatefulSet", "StatfulSet", 1},     // missing 'e'
		{"ConfigMap", "configmap", 2},        // two case diffs
		{"Pod", "Pode", 1},                   // extra char
		{"Service", "Servce", 1},             // missing 'i'
		{"completely", "different", 8},       // very different
	}
	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := levenshtein(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestResolveShortName(t *testing.T) {
	tests := []struct {
		input string
		want  string
		found bool
	}{
		{"deploy", "Deployment", true},
		{"Deploy", "Deployment", true},
		{"DEPLOY", "Deployment", true},
		{"svc", "Service", true},
		{"sts", "StatefulSet", true},
		{"ds", "DaemonSet", true},
		{"rs", "ReplicaSet", true},
		{"cm", "ConfigMap", true},
		{"sa", "ServiceAccount", true},
		{"pvc", "PersistentVolumeClaim", true},
		{"pv", "PersistentVolume", true},
		{"ns", "Namespace", true},
		{"no", "Node", true},
		{"po", "Pod", true},
		{"ing", "Ingress", true},
		{"ep", "Endpoints", true},
		{"hpa", "HorizontalPodAutoscaler", true},
		{"cj", "CronJob", true},
		{"Deployment", "", false},  // full names don't match
		{"xyz", "", false},          // unknown short name
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, found := resolveShortName(tt.input)
			if found != tt.found {
				t.Errorf("resolveShortName(%q) found = %v, want %v", tt.input, found, tt.found)
			}
			if got != tt.want {
				t.Errorf("resolveShortName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSuggestKind(t *testing.T) {
	knownKinds := []string{
		"Pod", "Service", "Deployment", "StatefulSet", "DaemonSet",
		"ReplicaSet", "ConfigMap", "Secret", "Namespace", "Node",
		"Ingress", "Job", "CronJob", "ServiceAccount",
	}

	tests := []struct {
		input string
		want  string
	}{
		{"Deploymnt", "Deployment"},
		{"Deploment", "Deployment"},
		{"deploymnet", "Deployment"},
		{"StatfulSet", "StatefulSet"},
		{"Servce", "Service"},
		{"Configmap", "ConfigMap"},
		{"secrt", "Secret"},
		{"Pode", "Pod"},
		{"Nod", "Pod"},  // distance 1 from both Pod and Node; Pod wins alphabetically
		{"completely_unknown_xyz", ""},  // too far from anything
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := suggestKind(tt.input, knownKinds)
			if got != tt.want {
				t.Errorf("suggestKind(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
