package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// requireStringOrJSON returns a string argument by key. If the LLM sends a
// JSON object instead of a string, it marshals the object to a JSON string.
// This handles the common case where a tool parameter is described as
// "a JSON string" but the LLM passes a raw object.
func requireStringOrJSON(req mcp.CallToolRequest, key string) (string, error) {
	args := req.GetArguments()
	val, ok := args[key]
	if !ok {
		return "", fmt.Errorf("required argument %q not found", key)
	}
	if str, isStr := val.(string); isStr {
		return str, nil
	}
	// The value is a JSON object/array/number — marshal it.
	b, err := json.Marshal(val)
	if err != nil {
		return "", fmt.Errorf("argument %q could not be serialized: %w", key, err)
	}
	return string(b), nil
}

// dryRunOption returns the DryRun field value for K8s API options.
func dryRunOption(dryRun bool) []string {
	if dryRun {
		return []string{metav1.DryRunAll}
	}
	return nil
}

// resolveFieldValidation maps the user-facing validate parameter to the
// Kubernetes API fieldValidation value. Accepted inputs (case-insensitive):
//
//	"strict"       → "Strict"  (default)
//	"warn"         → "Warn"
//	"ignore"/"none" → "Ignore"
func resolveFieldValidation(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "strict", "":
		return "Strict", nil
	case "warn":
		return "Warn", nil
	case "ignore", "none":
		return "Ignore", nil
	default:
		return "", fmt.Errorf("invalid validate value %q: must be one of strict, warn, ignore, none", v)
	}
}

// parseDuration parses a human-friendly duration string like "5m", "1h", "30s", "2d".
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	// Handle "d" suffix for days (not supported by time.ParseDuration).
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return d, nil
}
