package mcplog

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// LoggerName is the MCP logger name used in all notifications.
const LoggerName = "kubectl-mcp"

// NewNotification creates an MCP logging notification with our logger name.
func NewNotification(level mcp.LoggingLevel, data any) mcp.LoggingMessageNotification {
	return mcp.NewLoggingMessageNotification(level, LoggerName, data)
}
