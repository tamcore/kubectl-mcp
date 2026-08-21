package mcplog

import (
	"errors"

	"github.com/mark3labs/mcp-go/mcp"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// LoggerName is the MCP logger name used in all notifications.
const LoggerName = "kubectl-mcp"

// NewNotification creates an MCP logging notification with our logger name.
func NewNotification(level mcp.LoggingLevel, data any) mcp.LoggingMessageNotification {
	return mcp.NewLoggingMessageNotification(level, LoggerName, data)
}

// ClassifyK8sError inspects a Kubernetes API error and returns an appropriate
// MCP log level and human-readable message. Returns empty strings if the error
// is not a Kubernetes StatusError.
func ClassifyK8sError(err error) (mcp.LoggingLevel, string) {
	if err == nil {
		return "", ""
	}

	if _, ok := errors.AsType[*k8serrors.StatusError](err); !ok {
		return "", ""
	}

	switch {
	case k8serrors.IsNotFound(err):
		return mcp.LoggingLevelInfo, "Resource not found — it may not exist or may have been deleted"
	case k8serrors.IsForbidden(err):
		return mcp.LoggingLevelError, "Permission denied — check RBAC permissions"
	case k8serrors.IsUnauthorized(err):
		return mcp.LoggingLevelError, "Authentication failed — check cluster credentials"
	case k8serrors.IsAlreadyExists(err):
		return mcp.LoggingLevelWarning, "Resource already exists"
	case k8serrors.IsInvalid(err):
		return mcp.LoggingLevelError, "Invalid resource specification — check resource definition"
	case k8serrors.IsConflict(err):
		return mcp.LoggingLevelError, "Resource conflict — resource may have been modified concurrently"
	case k8serrors.IsTimeout(err):
		return mcp.LoggingLevelError, "Request timeout — cluster may be slow or overloaded"
	case k8serrors.IsServerTimeout(err):
		return mcp.LoggingLevelError, "Server timeout — cluster may be slow or overloaded"
	case k8serrors.IsServiceUnavailable(err):
		return mcp.LoggingLevelError, "Service unavailable — cluster may be unreachable"
	case k8serrors.IsTooManyRequests(err):
		return mcp.LoggingLevelWarning, "Rate limited — too many requests to the cluster"
	default:
		return mcp.LoggingLevelError, "Operation failed — cluster may be unreachable or experiencing issues"
	}
}
