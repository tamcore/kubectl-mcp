package resources

import (
	"testing"
)

func TestParseK8sURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    ParsedURI
		wantErr bool
	}{
		{
			name: "namespaced core resource (pod)",
			uri:  "k8s://my-cluster/namespaces/default/core/v1/pods/nginx",
			want: ParsedURI{
				Context:   "my-cluster",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
				Resource:  "pods",
				Name:      "nginx",
			},
		},
		{
			name: "namespaced core resource (configmap)",
			uri:  "k8s://prod/namespaces/kube-system/core/v1/configmaps/coredns",
			want: ParsedURI{
				Context:   "prod",
				Namespace: "kube-system",
				Group:     "",
				Version:   "v1",
				Resource:  "configmaps",
				Name:      "coredns",
			},
		},
		{
			name: "namespaced apps group resource (deployment)",
			uri:  "k8s://dev/namespaces/default/apps/v1/deployments/web",
			want: ParsedURI{
				Context:   "dev",
				Namespace: "default",
				Group:     "apps",
				Version:   "v1",
				Resource:  "deployments",
				Name:      "web",
			},
		},
		{
			name: "cluster-scoped core resource (node)",
			uri:  "k8s://my-cluster/core/v1/nodes/worker-1",
			want: ParsedURI{
				Context:  "my-cluster",
				Group:    "",
				Version:  "v1",
				Resource: "nodes",
				Name:     "worker-1",
			},
		},
		{
			name: "cluster-scoped resource (namespace)",
			uri:  "k8s://my-cluster/core/v1/namespaces/kube-system",
			want: ParsedURI{
				Context:  "my-cluster",
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
				Name:     "kube-system",
			},
		},
		{
			name: "cluster-scoped non-core resource (clusterrole)",
			uri:  "k8s://staging/rbac.authorization.k8s.io/v1/clusterroles/admin",
			want: ParsedURI{
				Context:  "staging",
				Group:    "rbac.authorization.k8s.io",
				Version:  "v1",
				Resource: "clusterroles",
				Name:     "admin",
			},
		},
		{
			name: "namespaced secret",
			uri:  "k8s://my-cluster/namespaces/default/core/v1/secrets/my-secret",
			want: ParsedURI{
				Context:   "my-cluster",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
				Resource:  "secrets",
				Name:      "my-secret",
			},
		},
		{
			name:    "wrong scheme",
			uri:     "http://my-cluster/core/v1/nodes/worker-1",
			wantErr: true,
		},
		{
			name:    "too few segments",
			uri:     "k8s://my-cluster/v1/nodes",
			wantErr: true,
		},
		{
			name:    "empty segment in URI",
			uri:     "k8s://my-cluster//v1/nodes/worker-1",
			wantErr: true,
		},
		{
			name:    "namespaced with wrong segment count",
			uri:     "k8s://my-cluster/namespaces/default/core/v1",
			wantErr: true,
		},
		{
			name:    "cluster-scoped with extra segment",
			uri:     "k8s://my-cluster/core/v1/nodes/worker-1/extra",
			wantErr: true,
		},
		{
			name:    "empty URI",
			uri:     "",
			wantErr: true,
		},
		{
			name:    "just scheme",
			uri:     "k8s://",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseK8sURI(tt.uri)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseK8sURI(%q) expected error, got nil", tt.uri)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseK8sURI(%q) unexpected error: %v", tt.uri, err)
			}
			if got != tt.want {
				t.Errorf("ParseK8sURI(%q)\n  got:  %+v\n  want: %+v", tt.uri, got, tt.want)
			}
		})
	}
}
