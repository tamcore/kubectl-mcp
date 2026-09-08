package mcplog

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestNewNotification(t *testing.T) {
	n := NewNotification(mcp.LoggingLevelInfo, "test message")
	if n.Params.Level != mcp.LoggingLevelInfo {
		t.Errorf("level = %q, want %q", n.Params.Level, mcp.LoggingLevelInfo)
	}
	if n.Params.Logger != LoggerName {
		t.Errorf("logger = %q, want %q", n.Params.Logger, LoggerName)
	}
	data, ok := n.Params.Data.(string)
	if !ok || data != "test message" {
		t.Errorf("data = %v, want %q", n.Params.Data, "test message")
	}
}
