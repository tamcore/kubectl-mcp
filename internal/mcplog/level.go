package mcplog

import (
	"fmt"
	"strings"
)

// LogLevel controls the verbosity of server-side logging.
type LogLevel string

const (
	// LogLevelOff disables all logging (stderr and MCP notifications).
	LogLevelOff LogLevel = "off"
	// LogLevelInfo logs tool call summaries and errors (default).
	LogLevelInfo LogLevel = "info"
	// LogLevelDebug logs full arguments, results, and K8s HTTP details.
	LogLevelDebug LogLevel = "debug"
)

// String returns the log level as a lowercase string.
func (l LogLevel) String() string {
	return string(l)
}

// ParseLogLevel parses a string into a LogLevel.
// Accepted values (case-insensitive): "off", "info", "debug".
func ParseLogLevel(s string) (LogLevel, error) {
	switch strings.ToLower(s) {
	case "off":
		return LogLevelOff, nil
	case "info":
		return LogLevelInfo, nil
	case "debug":
		return LogLevelDebug, nil
	default:
		return "", fmt.Errorf("invalid log level %q: must be off, info, or debug", s)
	}
}
