package tools

import "testing"

func TestFormatPortSpec(t *testing.T) {
	tests := []struct {
		name       string
		localPort  uint16
		remotePort uint16
		want       string
	}{
		{"auto_assign", 0, 8080, "0:8080"},
		{"explicit_port", 9090, 8080, "9090:8080"},
		{"same_port", 80, 80, "80:80"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatPortSpec(tt.localPort, tt.remotePort)
			if got != tt.want {
				t.Errorf("formatPortSpec(%d, %d) = %q, want %q", tt.localPort, tt.remotePort, got, tt.want)
			}
		})
	}
}
