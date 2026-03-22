//go:build e2e

package e2e

import (
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestLogLevel(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("off_no_notifications", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.LogLevel = "off"

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				var mu sync.Mutex
				var notifications []mcp.JSONRPCNotification
				c.OnNotification(func(n mcp.JSONRPCNotification) {
					mu.Lock()
					defer mu.Unlock()
					notifications = append(notifications, n)
				})

				result := callTool(t, c, "list_contexts", nil)
				if result.IsError {
					t.Fatalf("list_contexts failed: %s", resultText(result))
				}

				mu.Lock()
				logNotifs := filterLogNotifications(notifications)
				mu.Unlock()

				if len(logNotifs) > 0 {
					t.Errorf("expected no logging notifications at off level, got %d", len(logNotifs))
				}
			})

			t.Run("info_no_debug_notifications", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.LogLevel = "info"

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				var mu sync.Mutex
				var notifications []mcp.JSONRPCNotification
				c.OnNotification(func(n mcp.JSONRPCNotification) {
					mu.Lock()
					defer mu.Unlock()
					notifications = append(notifications, n)
				})

				result := callTool(t, c, "list_contexts", nil)
				if result.IsError {
					t.Fatalf("list_contexts failed: %s", resultText(result))
				}

				mu.Lock()
				logNotifs := filterLogNotifications(notifications)
				mu.Unlock()

				for _, n := range logNotifs {
					level := extractNotificationLevel(n)
					if level == "debug" {
						t.Errorf("unexpected debug notification at info level: %+v", n)
					}
				}
			})

			t.Run("debug_sends_notifications", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.LogLevel = "debug"

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				var mu sync.Mutex
				var notifications []mcp.JSONRPCNotification
				c.OnNotification(func(n mcp.JSONRPCNotification) {
					mu.Lock()
					defer mu.Unlock()
					notifications = append(notifications, n)
				})

				result := callTool(t, c, "list_contexts", nil)
				if result.IsError {
					t.Fatalf("list_contexts failed: %s", resultText(result))
				}

				mu.Lock()
				logNotifs := filterLogNotifications(notifications)
				mu.Unlock()

				if len(logNotifs) == 0 {
					t.Error("expected logging notifications at debug level, got none")
				}
			})

			t.Run("tools_work_at_all_levels", func(t *testing.T) {
				for _, level := range []string{"off", "info", "debug"} {
					t.Run(level, func(t *testing.T) {
						cfg := defaultConfig()
						cfg.LogLevel = level

						base := tc.startFunc(t, cfg)
						c := tc.clientFunc(t, base)

						result := callTool(t, c, "list_contexts", nil)
						if result.IsError {
							t.Fatalf("list_contexts failed at %s level: %s", level, resultText(result))
						}
						if resultText(result) == "" {
							t.Fatalf("expected non-empty result at %s level", level)
						}
					})
				}
			})
		})
	}
}

// filterLogNotifications returns only notifications/message notifications.
func filterLogNotifications(notifs []mcp.JSONRPCNotification) []mcp.JSONRPCNotification {
	var result []mcp.JSONRPCNotification
	for _, n := range notifs {
		if n.Method == "notifications/message" {
			result = append(result, n)
		}
	}
	return result
}

// extractNotificationLevel extracts the logging level from a notification params.
func extractNotificationLevel(n mcp.JSONRPCNotification) string {
	level, _ := n.Params.AdditionalFields["level"].(string)
	return level
}
