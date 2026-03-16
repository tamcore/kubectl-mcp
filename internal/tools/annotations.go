package tools

import (
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// filterObjAnnotations applies annotation filtering to a single unstructured
// object based on the include/exclude patterns from the request.
func filterObjAnnotations(obj *unstructured.Unstructured, req mcp.CallToolRequest) {
	include := parseCSV(req.GetString("include_annotations", ""))
	exclude := parseCSV(req.GetString("exclude_annotations", ""))

	annotations := obj.GetAnnotations()
	filtered := kube.FilterAnnotations(annotations, include, exclude)
	if filtered == nil {
		return
	}
	obj.SetAnnotations(filtered)
}

// filterListAnnotations applies annotation filtering to every item in a list.
func filterListAnnotations(list *unstructured.UnstructuredList, req mcp.CallToolRequest) {
	for i := range list.Items {
		filterObjAnnotations(&list.Items[i], req)
	}
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
