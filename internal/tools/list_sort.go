package tools

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// parseSortBy parses a sortBy string into a field path and a descending flag.
// A leading "-" means descending. A leading "." is optional and stripped.
// Returns the normalised dot-joined field path (no leading dot) and descending.
func parseSortBy(raw string) (fieldPath string, descending bool) {
	s := raw
	if strings.HasPrefix(s, "-") {
		descending = true
		s = s[1:]
	}
	s = strings.TrimPrefix(s, ".")
	return s, descending
}

// extractSortKey extracts a comparable string sort key from an unstructured
// object at the given dot-separated field path.
// For RFC3339 timestamps it returns a zero-padded representation so that
// lexicographic ordering equals chronological ordering.
// Returns ("", false) when the field is absent.
func extractSortKey(obj map[string]interface{}, fieldPath string) (string, bool) {
	parts := strings.Split(fieldPath, ".")
	raw, found := nestedFieldValue(obj, parts)
	if !found {
		return "", false
	}

	// Try RFC3339 timestamp parsing for correct time ordering.
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		// Format as RFC3339Nano so lexicographic order == chronological order.
		return t.UTC().Format(time.RFC3339Nano), true
	}

	return raw, true
}

// sortUnstructured sorts items in place by the given dot-separated field path.
// It returns an error when the field path is not found in any item (and at
// least one item exists), because that is almost certainly a typo.
func sortUnstructured(items []unstructured.Unstructured, fieldPath string, descending bool) error {
	if len(items) == 0 {
		return nil
	}

	// Verify at least one item has the field; fail early for unknown paths.
	foundAny := false
	for _, item := range items {
		if _, ok := extractSortKey(item.Object, fieldPath); ok {
			foundAny = true
			break
		}
	}
	if !foundAny {
		return fmt.Errorf("sortBy field %q not found in any item", fieldPath)
	}

	sort.SliceStable(items, func(i, j int) bool {
		ki, _ := extractSortKey(items[i].Object, fieldPath)
		kj, _ := extractSortKey(items[j].Object, fieldPath)
		if descending {
			return ki > kj
		}
		return ki < kj
	})
	return nil
}
