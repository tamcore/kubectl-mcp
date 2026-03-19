package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// ---------------------------------------------------------------------------
// list_rbac_bindings tests
// ---------------------------------------------------------------------------

func TestListRBACBindings_ClusterRoleBindings(t *testing.T) {
	fakeCS := fake.NewClientset(
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-admin-binding"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "cluster-admin", APIGroup: "rbac.authorization.k8s.io"},
			Subjects: []rbacv1.Subject{
				{Kind: "User", Name: "admin@example.com", APIGroup: "rbac.authorization.k8s.io"},
			},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "view-binding"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "view", APIGroup: "rbac.authorization.k8s.io"},
			Subjects: []rbacv1.Subject{
				{Kind: "Group", Name: "developers", APIGroup: "rbac.authorization.k8s.io"},
			},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_bindings", func(s *server.MCPServer) {
		registerListRBACBindings(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON array, got: %s", text)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 ClusterRoleBindings, got %d: %s", len(items), text)
	}
	for _, item := range items {
		if item["kind"] != "ClusterRoleBinding" {
			t.Errorf("expected kind=ClusterRoleBinding, got %v", item["kind"])
		}
	}
}

func TestListRBACBindings_RoleBindings(t *testing.T) {
	fakeCS := fake.NewClientset(
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "edit-binding", Namespace: "default"},
			RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "editor", APIGroup: "rbac.authorization.k8s.io"},
			Subjects: []rbacv1.Subject{
				{Kind: "ServiceAccount", Name: "my-sa", Namespace: "default"},
			},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "view-binding", Namespace: "other-ns"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "view", APIGroup: "rbac.authorization.k8s.io"},
			Subjects: []rbacv1.Subject{
				{Kind: "User", Name: "bob", APIGroup: "rbac.authorization.k8s.io"},
			},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_bindings", func(s *server.MCPServer) {
		registerListRBACBindings(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON array, got: %s", text)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 RoleBinding in default, got %d: %s", len(items), text)
	}
	if items[0]["kind"] != "RoleBinding" {
		t.Errorf("expected kind=RoleBinding, got %v", items[0]["kind"])
	}
	if items[0]["name"] != "edit-binding" {
		t.Errorf("expected name=edit-binding, got %v", items[0]["name"])
	}
}

func TestListRBACBindings_SubjectFilter(t *testing.T) {
	fakeCS := fake.NewClientset(
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "sa-binding"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "editor"},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "target-sa", Namespace: "default"}},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "other-binding"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "viewer"},
			Subjects:   []rbacv1.Subject{{Kind: "User", Name: "alice"}},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_bindings", func(s *server.MCPServer) {
		registerListRBACBindings(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"subject": "target-sa",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON, got: %s", text)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item after subject filter, got %d: %s", len(items), text)
	}
	if items[0]["name"] != "sa-binding" {
		t.Errorf("expected sa-binding, got %v", items[0]["name"])
	}
}

func TestListRBACBindings_SubjectKindFilter(t *testing.T) {
	fakeCS := fake.NewClientset(
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "user-binding"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "editor"},
			Subjects:   []rbacv1.Subject{{Kind: "User", Name: "alice"}},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "sa-binding"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "viewer"},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "my-sa", Namespace: "default"}},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_bindings", func(s *server.MCPServer) {
		registerListRBACBindings(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"subjectKind": "ServiceAccount",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON, got: %s", text)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item after subjectKind filter, got %d: %s", len(items), text)
	}
	if items[0]["name"] != "sa-binding" {
		t.Errorf("expected sa-binding, got %v", items[0]["name"])
	}
}

func TestListRBACBindings_Empty(t *testing.T) {
	fakeCS := fake.NewClientset()
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_bindings", func(s *server.MCPServer) {
		registerListRBACBindings(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if text != "[]" {
		t.Errorf("expected empty array, got: %s", text)
	}
}

func TestListRBACBindings_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}}
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_bindings", func(s *server.MCPServer) {
		registerListRBACBindings(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resultText(t, res), "not allowed") {
		t.Errorf("expected not-allowed error, got: %s", resultText(t, res))
	}
}

// ---------------------------------------------------------------------------
// list_rbac_roles tests
// ---------------------------------------------------------------------------

func TestListRBACRoles_ClusterRoles(t *testing.T) {
	fakeCS := fake.NewClientset(
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "admin"},
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
			},
		},
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "view"},
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
			},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_roles", func(s *server.MCPServer) {
		registerListRBACRoles(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON array, got: %s", text)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 ClusterRoles, got %d", len(items))
	}
}

func TestListRBACRoles_Roles(t *testing.T) {
	fakeCS := fake.NewClientset(
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-reader", Namespace: "default"},
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
			},
		},
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "secret-reader", Namespace: "other-ns"},
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
			},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_roles", func(s *server.MCPServer) {
		registerListRBACRoles(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON, got: %s", text)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 Role in default, got %d", len(items))
	}
	if items[0]["name"] != "pod-reader" {
		t.Errorf("expected pod-reader, got %v", items[0]["name"])
	}
}

func TestListRBACRoles_NamedClusterRole(t *testing.T) {
	fakeCS := fake.NewClientset(
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "custom-role"},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     []string{"get", "list", "create"},
				},
			},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_roles", func(s *server.MCPServer) {
		registerListRBACRoles(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"name": "custom-role",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON object, got: %s", text)
	}
	if result["name"] != "custom-role" {
		t.Errorf("expected name=custom-role, got %v", result["name"])
	}
	rules, ok := result["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		t.Errorf("expected rules in detailed output, got: %v", result["rules"])
	}
}

func TestListRBACRoles_NamedRole(t *testing.T) {
	fakeCS := fake.NewClientset(
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-reader", Namespace: "default"},
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
			},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_roles", func(s *server.MCPServer) {
		registerListRBACRoles(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "pod-reader",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON object, got: %s", text)
	}
	if result["name"] != "pod-reader" {
		t.Errorf("expected name=pod-reader, got %v", result["name"])
	}
	if result["kind"] != "Role" {
		t.Errorf("expected kind=Role, got %v", result["kind"])
	}
}

func TestListRBACRoles_Empty(t *testing.T) {
	fakeCS := fake.NewClientset()
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_roles", func(s *server.MCPServer) {
		registerListRBACRoles(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if text != "[]" {
		t.Errorf("expected empty array, got: %s", text)
	}
}

func TestListRBACRoles_NotFound(t *testing.T) {
	fakeCS := fake.NewClientset()
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_rbac_roles", func(s *server.MCPServer) {
		registerListRBACRoles(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"name": "nonexistent",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Errorf("expected error for not-found role, got: %s", resultText(t, res))
	}
}

// ---------------------------------------------------------------------------
// list_service_accounts tests
// ---------------------------------------------------------------------------

func TestListServiceAccounts_List(t *testing.T) {
	fakeCS := fake.NewClientset(
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "my-sa", Namespace: "default"},
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "other-sa", Namespace: "kube-system"},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_service_accounts", func(s *server.MCPServer) {
		registerListServiceAccounts(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON array, got: %s", text)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 SAs in default, got %d: %s", len(items), text)
	}
}

func TestListServiceAccounts_AllNamespaces(t *testing.T) {
	fakeCS := fake.NewClientset(
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "sa1", Namespace: "ns1"},
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "sa2", Namespace: "ns2"},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_service_accounts", func(s *server.MCPServer) {
		registerListServiceAccounts(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON array, got: %s", text)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 SAs across all namespaces, got %d", len(items))
	}
}

func TestListServiceAccounts_Named(t *testing.T) {
	fakeCS := fake.NewClientset(
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-sa",
				Namespace: "default",
			},
			Secrets: []corev1.ObjectReference{
				{Name: "my-sa-token-xyz"},
			},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_service_accounts", func(s *server.MCPServer) {
		registerListServiceAccounts(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "my-sa",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON object, got: %s", text)
	}
	if result["name"] != "my-sa" {
		t.Errorf("expected name=my-sa, got %v", result["name"])
	}
	// Secret names should be visible, but not data.
	secrets, ok := result["secrets"].([]interface{})
	if !ok || len(secrets) != 1 {
		t.Errorf("expected 1 secret name, got: %v", result["secrets"])
	}
	if secrets[0] != "my-sa-token-xyz" {
		t.Errorf("expected secret name my-sa-token-xyz, got %v", secrets[0])
	}
}

func TestListServiceAccounts_NoTokenData(t *testing.T) {
	// Ensure secret data is never exposed, only names are shown.
	fakeCS := fake.NewClientset(
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "token-sa", Namespace: "default"},
			Secrets:    []corev1.ObjectReference{{Name: "token-sa-token-abc"}},
		},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_service_accounts", func(s *server.MCPServer) {
		registerListServiceAccounts(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "token-sa",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	// Output should NOT contain any "data" or "token" values, only the secret name.
	if strings.Contains(text, `"data"`) {
		t.Error("secret data should never be exposed in SA output")
	}
}

func TestListServiceAccounts_Empty(t *testing.T) {
	fakeCS := fake.NewClientset()
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_service_accounts", func(s *server.MCPServer) {
		registerListServiceAccounts(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if text != "[]" {
		t.Errorf("expected empty array, got: %s", text)
	}
}

func TestListServiceAccounts_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}}
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)
	handler := getHandler(t, "list_service_accounts", func(s *server.MCPServer) {
		registerListServiceAccounts(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resultText(t, res), "not allowed") {
		t.Errorf("expected not-allowed error")
	}
}
