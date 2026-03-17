package tools

import (
	"strings"
	"testing"
)

func TestFormatNodeStats(t *testing.T) {
	rawJSON := `{
		"node": {
			"nodeName": "test-node",
			"cpu": {
				"usageNanoCores": 250000000,
				"usageCoreNanoSeconds": 123456789
			},
			"memory": {
				"availableBytes": 2147483648,
				"usageBytes": 4294967296,
				"workingSetBytes": 3221225472,
				"rssBytes": 1073741824
			},
			"fs": {
				"availableBytes": 53687091200,
				"capacityBytes": 107374182400,
				"usedBytes": 53687091200
			}
		}
	}`

	result, err := formatNodeStats("test-node", []byte(rawJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []string{
		"Node: test-node",
		"CPU:",
		"Usage: 250.0m",
		"Memory:",
		"Working Set: 3.0Gi",
		"Usage:       4.0Gi",
		"Available:   2.0Gi",
		"RSS:         1.0Gi",
		"Filesystem:",
		"Capacity:  100.0Gi",
		"Used:      50.0Gi",
		"Available: 50.0Gi",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected %q in output, got:\n%s", check, result)
		}
	}
}

func TestFormatNodeStats_MissingFields(t *testing.T) {
	rawJSON := `{
		"node": {
			"nodeName": "test-node",
			"cpu": {},
			"memory": {}
		}
	}`

	result, err := formatNodeStats("test-node", []byte(rawJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "<unavailable>") {
		t.Errorf("expected <unavailable> for missing CPU, got:\n%s", result)
	}
}

func TestFormatNodeStats_InvalidJSON(t *testing.T) {
	_, err := formatNodeStats("test-node", []byte("{invalid"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0Ki"},
		{1048576, "1.0Mi"},
		{1073741824, "1.0Gi"},
		{2147483648, "2.0Gi"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.input)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %s, want %s", tt.input, result, tt.expected)
		}
	}
}
