package mcplog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestDefaultLogPath(t *testing.T) {
	got := DefaultLogPath()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	want := filepath.Join(home, ".kubectl-mcp", fmt.Sprintf("server-%d.log", os.Getpid()))
	if got != want {
		t.Fatalf("DefaultLogPath() = %q, want %q", got, want)
	}
}

func TestDefaultLogPath_ContainsPID(t *testing.T) {
	got := DefaultLogPath()
	pid := fmt.Sprintf("%d", os.Getpid())
	if !strings.Contains(got, pid) {
		t.Fatalf("DefaultLogPath() = %q, expected to contain PID %s", got, pid)
	}
}

func TestDefaultLogPath_ContainsDotKubectlMcp(t *testing.T) {
	got := DefaultLogPath()
	if !strings.Contains(got, ".kubectl-mcp") {
		t.Fatalf("DefaultLogPath() = %q, expected to contain .kubectl-mcp", got)
	}
	if !strings.HasSuffix(got, ".log") {
		t.Fatalf("DefaultLogPath() = %q, expected to end with .log", got)
	}
}

func TestDefaultLogDir(t *testing.T) {
	got := DefaultLogDir()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	want := filepath.Join(home, ".kubectl-mcp")
	if got != want {
		t.Fatalf("DefaultLogDir() = %q, want %q", got, want)
	}
}

func TestDefaultLogDir_ContainsDotKubectlMcp(t *testing.T) {
	got := DefaultLogDir()
	if !strings.Contains(got, ".kubectl-mcp") {
		t.Fatalf("DefaultLogDir() = %q, expected to contain .kubectl-mcp", got)
	}
}
