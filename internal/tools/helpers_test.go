package tools

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{name: "minutes", input: "5m", want: 5 * time.Minute},
		{name: "hours", input: "1h", want: time.Hour},
		{name: "seconds", input: "30s", want: 30 * time.Second},
		{name: "days", input: "2d", want: 48 * time.Hour},
		{name: "milliseconds", input: "500ms", want: 500 * time.Millisecond},
		{name: "zero seconds", input: "0s", want: 0},
		{name: "one day", input: "1d", want: 24 * time.Hour},
		{name: "whitespace trimmed", input: "  10s  ", want: 10 * time.Second},
		{name: "invalid unit", input: "5x", wantErr: true},
		{name: "alpha only", input: "abc", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
		{name: "invalid day number", input: "abcd", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
