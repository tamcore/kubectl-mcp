package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// ---------------------------------------------------------------------------
// list_rbac_bindings
// ---------------------------------------------------------------------------

func registerListRBACBindings(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("list_rbac_bindings",
		mcp.WithDescription("List RBAC bindings (ClusterRoleBindings or RoleBindings). Omit namespace for cluster-wide ClusterRoleBindings."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace to list RoleBindings (omit for ClusterRoleBindings)"),
		),
		mcp.WithString("subject",
			mcp.Description("Filter by subject name (user, group, or service account name)"),
		),
		mcp.WithString("subjectKind",
			mcp.Description("Filter by subject kind: User, Group, or ServiceAccount"),
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
		subject := req.GetString("subject", "")
		subjectKind := req.GetString("subjectKind", "")

		type bindingItem struct {
			Kind      string      `json:"kind"`
			Name      string      `json:"name"`
			Namespace string      `json:"namespace,omitempty"`
			RoleRef   interface{} `json:"roleRef"`
			Subjects  interface{} `json:"subjects"`
		}

		var items []bindingItem

		if namespace == "" {
			list, err := cc.Clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list ClusterRoleBindings: %v", err)), nil
			}
			for _, crb := range list.Items {
				if !subjectsMatch(crb.Subjects, subject, subjectKind) {
					continue
				}
				items = append(items, bindingItem{
					Kind:     "ClusterRoleBinding",
					Name:     crb.Name,
					RoleRef:  crb.RoleRef,
					Subjects: crb.Subjects,
				})
			}
		} else {
			list, err := cc.Clientset.RbacV1().RoleBindings(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list RoleBindings: %v", err)), nil
			}
			for _, rb := range list.Items {
				if !subjectsMatch(rb.Subjects, subject, subjectKind) {
					continue
				}
				items = append(items, bindingItem{
					Kind:      "RoleBinding",
					Name:      rb.Name,
					Namespace: rb.Namespace,
					RoleRef:   rb.RoleRef,
					Subjects:  rb.Subjects,
				})
			}
		}

		if items == nil {
			return mcp.NewToolResultText("[]"), nil
		}

		out, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	})
}

// subjectsMatch returns true if all non-empty filters match at least one subject.
func subjectsMatch(subjects interface{}, name, kind string) bool {
	if name == "" && kind == "" {
		return true
	}
	// Accept both rbacv1.Subject slices (passed from typed clients).
	// We marshal/unmarshal to handle generic Subject checks.
	b, err := json.Marshal(subjects)
	if err != nil {
		return false
	}
	var list []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &list); err != nil {
		return false
	}
	for _, s := range list {
		nameMatch := name == "" || s.Name == name
		kindMatch := kind == "" || s.Kind == kind
		if nameMatch && kindMatch {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// list_rbac_roles
// ---------------------------------------------------------------------------

func registerListRBACRoles(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("list_rbac_roles",
		mcp.WithDescription("List or get RBAC roles (ClusterRoles or Roles). Omit namespace for ClusterRoles. Provide name for detailed view with rules."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace to list Roles (omit for ClusterRoles)"),
		),
		mcp.WithString("name",
			mcp.Description("Name of a specific role or cluster role to get details for"),
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

		// Named get returns a single detailed object.
		if name != "" {
			if namespace != "" {
				role, err := cc.Clientset.RbacV1().Roles(namespace).Get(ctx, name, metav1.GetOptions{})
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("role %q not found in namespace %q: %v", name, namespace, err)), nil
				}
				result := map[string]interface{}{
					"kind":      "Role",
					"name":      role.Name,
					"namespace": role.Namespace,
					"rules":     role.Rules,
				}
				out, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return mcp.NewToolResultText(string(out)), nil
			}
			clusterRole, err := cc.Clientset.RbacV1().ClusterRoles().Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("cluster role %q not found: %v", name, err)), nil
			}
			result := map[string]interface{}{
				"kind":  "ClusterRole",
				"name":  clusterRole.Name,
				"rules": clusterRole.Rules,
			}
			out, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(string(out)), nil
		}

		type roleItem struct {
			Kind      string `json:"kind"`
			Name      string `json:"name"`
			Namespace string `json:"namespace,omitempty"`
		}

		var items []roleItem

		if namespace == "" {
			list, err := cc.Clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list ClusterRoles: %v", err)), nil
			}
			for _, cr := range list.Items {
				items = append(items, roleItem{Kind: "ClusterRole", Name: cr.Name})
			}
		} else {
			list, err := cc.Clientset.RbacV1().Roles(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list Roles: %v", err)), nil
			}
			for _, r := range list.Items {
				items = append(items, roleItem{Kind: "Role", Name: r.Name, Namespace: r.Namespace})
			}
		}

		if items == nil {
			return mcp.NewToolResultText("[]"), nil
		}

		out, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	})
}

// ---------------------------------------------------------------------------
// list_service_accounts
// ---------------------------------------------------------------------------

func registerListServiceAccounts(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("list_service_accounts",
		mcp.WithDescription("List or get ServiceAccounts. Provides secret names but never exposes token data."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace to list ServiceAccounts (omit for all namespaces)"),
		),
		mcp.WithString("name",
			mcp.Description("Name of a specific ServiceAccount to get details for (requires namespace)"),
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

		// Named get: return SA detail with secret names only.
		if name != "" {
			sa, err := cc.Clientset.CoreV1().ServiceAccounts(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("service account %q not found: %v", name, err)), nil
			}
			secretNames := make([]string, 0, len(sa.Secrets))
			for _, ref := range sa.Secrets {
				secretNames = append(secretNames, ref.Name)
			}
			result := map[string]interface{}{
				"name":      sa.Name,
				"namespace": sa.Namespace,
				"secrets":   secretNames,
			}
			out, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(string(out)), nil
		}

		type saItem struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		}

		list, err := cc.Clientset.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list ServiceAccounts: %v", err)), nil
		}

		if len(list.Items) == 0 {
			return mcp.NewToolResultText("[]"), nil
		}

		items := make([]saItem, 0, len(list.Items))
		for _, sa := range list.Items {
			items = append(items, saItem{Name: sa.Name, Namespace: sa.Namespace})
		}

		out, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	})
}
