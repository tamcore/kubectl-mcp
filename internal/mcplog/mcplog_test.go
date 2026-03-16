package mcplog

import (
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func statusError(reason metav1.StatusReason, code int32) error {
	return &k8serrors.StatusError{
		ErrStatus: metav1.Status{
			Reason: reason,
			Code:   code,
		},
	}
}

func TestClassifyK8sError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantLevel mcp.LoggingLevel
		wantMsg   string
	}{
		{
			name:      "NotFound",
			err:       statusError(metav1.StatusReasonNotFound, 404),
			wantLevel: mcp.LoggingLevelInfo,
			wantMsg:   "not found",
		},
		{
			name:      "Forbidden",
			err:       statusError(metav1.StatusReasonForbidden, 403),
			wantLevel: mcp.LoggingLevelError,
			wantMsg:   "RBAC",
		},
		{
			name:      "Unauthorized",
			err:       statusError(metav1.StatusReasonUnauthorized, 401),
			wantLevel: mcp.LoggingLevelError,
			wantMsg:   "Authentication",
		},
		{
			name:      "AlreadyExists",
			err:       statusError(metav1.StatusReasonAlreadyExists, 409),
			wantLevel: mcp.LoggingLevelWarning,
			wantMsg:   "already exists",
		},
		{
			name:      "Invalid",
			err:       statusError(metav1.StatusReasonInvalid, 422),
			wantLevel: mcp.LoggingLevelError,
			wantMsg:   "Invalid",
		},
		{
			name:      "Conflict",
			err:       statusError(metav1.StatusReasonConflict, 409),
			wantLevel: mcp.LoggingLevelError,
			wantMsg:   "conflict",
		},
		{
			name:      "Timeout",
			err:       statusError(metav1.StatusReasonTimeout, 504),
			wantLevel: mcp.LoggingLevelError,
			wantMsg:   "timeout",
		},
		{
			name:      "ServerTimeout",
			err:       statusError(metav1.StatusReasonServerTimeout, 504),
			wantLevel: mcp.LoggingLevelError,
			wantMsg:   "timeout",
		},
		{
			name:      "ServiceUnavailable",
			err:       statusError(metav1.StatusReasonServiceUnavailable, 503),
			wantLevel: mcp.LoggingLevelError,
			wantMsg:   "unavailable",
		},
		{
			name:      "TooManyRequests",
			err:       statusError(metav1.StatusReasonTooManyRequests, 429),
			wantLevel: mcp.LoggingLevelWarning,
			wantMsg:   "Rate limited",
		},
		{
			name:      "Unknown status error",
			err:       statusError(metav1.StatusReasonGone, 410),
			wantLevel: mcp.LoggingLevelError,
			wantMsg:   "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, msg := ClassifyK8sError(tt.err)
			if level != tt.wantLevel {
				t.Errorf("level = %q, want %q", level, tt.wantLevel)
			}
			if msg == "" {
				t.Fatal("message should not be empty")
			}
			if !containsCI(msg, tt.wantMsg) {
				t.Errorf("message %q does not contain %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestClassifyK8sError_NonStatusError(t *testing.T) {
	level, msg := ClassifyK8sError(fmt.Errorf("some random error"))
	if level != "" {
		t.Errorf("expected empty level for non-status error, got %q", level)
	}
	if msg != "" {
		t.Errorf("expected empty message for non-status error, got %q", msg)
	}
}

func TestClassifyK8sError_Nil(t *testing.T) {
	level, msg := ClassifyK8sError(nil)
	if level != "" || msg != "" {
		t.Errorf("expected empty for nil error, got level=%q msg=%q", level, msg)
	}
}

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

// containsCI is a case-insensitive contains check.
func containsCI(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(substr) == 0 ||
		findCI(s, substr))
}

func findCI(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if eqFoldASCII(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func eqFoldASCII(a, b string) bool {
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
