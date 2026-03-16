package kube

import "path/filepath"

// DefaultExcludeAnnotations are annotation keys excluded from all resource
// output by default. These are typically large and noisy for LLM consumers.
var DefaultExcludeAnnotations = []string{
	"kubectl.kubernetes.io/last-applied-configuration",
}

// FilterAnnotations returns a new map containing only the annotations that
// survive the include/exclude filters. The original map is never modified.
//
// Logic:
//   - If include patterns are non-empty, only keys matching at least one
//     include pattern are kept.
//   - Keys matching any exclude pattern are removed.
//   - DefaultExcludeAnnotations are always appended to the exclude list.
func FilterAnnotations(annotations map[string]string, include, exclude []string) map[string]string {
	if annotations == nil {
		return nil
	}

	allExcludes := append(DefaultExcludeAnnotations, exclude...)

	result := make(map[string]string, len(annotations))
	for k, v := range annotations {
		if len(include) > 0 && !matchesAnyGlob(k, include) {
			continue
		}
		if matchesAnyGlob(k, allExcludes) {
			continue
		}
		result[k] = v
	}
	return result
}

func matchesAnyGlob(s string, patterns []string) bool {
	for _, p := range patterns {
		if matched, _ := filepath.Match(p, s); matched {
			return true
		}
	}
	return false
}
