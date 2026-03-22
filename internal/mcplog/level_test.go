package mcplog

import (
	"testing"
)

func TestParseLogLevel_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  LogLevel
	}{
		{"off", LogLevelOff},
		{"info", LogLevelInfo},
		{"debug", LogLevelDebug},
		{"OFF", LogLevelOff},
		{"Info", LogLevelInfo},
		{"DEBUG", LogLevelDebug},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseLogLevel(tt.input)
			if err != nil {
				t.Fatalf("ParseLogLevel(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseLogLevel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseLogLevel_Invalid(t *testing.T) {
	invalids := []string{"", "trace", "warn", "verbose", "1", " info"}
	for _, input := range invalids {
		t.Run(input, func(t *testing.T) {
			_, err := ParseLogLevel(input)
			if err == nil {
				t.Fatalf("ParseLogLevel(%q) expected error, got nil", input)
			}
		})
	}
}

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LogLevelOff, "off"},
		{LogLevelInfo, "info"},
		{LogLevelDebug, "debug"},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Fatalf("LogLevel.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
