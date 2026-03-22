package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tamcore/kubectl-mcp/internal/mcplog"
)

// Config holds all runtime configuration for the MCP server.
type Config struct {
	Transport        string
	SSEAddress       string
	HTTPAddress      string
	Kubeconfig       string
	Context          string
	AllowedContexts  []string
	DeniedContexts   []string
	AllowWrite       bool
	AllowDestructive bool
	AllowSecrets     bool
	AllowRaw         bool
	RateLimitRead    int
	RateLimitWrite   int
	LogLevel         string
}

// Validate checks the configuration for consistency.
func (c *Config) Validate() error {
	switch c.Transport {
	case "stdio", "sse", "streamable-http":
	default:
		return fmt.Errorf("invalid transport %q: must be stdio, sse, or streamable-http", c.Transport)
	}
	if c.AllowDestructive {
		c.AllowWrite = true
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if _, err := mcplog.ParseLogLevel(c.LogLevel); err != nil {
		return err
	}
	if c.RateLimitRead < 0 {
		return fmt.Errorf("invalid rate-limit-read %d: must be >= 0 (0 = unlimited)", c.RateLimitRead)
	}
	if c.RateLimitWrite < 0 {
		return fmt.Errorf("invalid rate-limit-write %d: must be >= 0 (0 = unlimited)", c.RateLimitWrite)
	}
	for _, p := range c.DeniedContexts {
		if isRegex(p) {
			if _, err := regexp.Compile(trimRegex(p)); err != nil {
				return fmt.Errorf("invalid denied-contexts regex %q: %w", p, err)
			}
		}
	}
	for _, p := range c.AllowedContexts {
		if isRegex(p) {
			if _, err := regexp.Compile(trimRegex(p)); err != nil {
				return fmt.Errorf("invalid allowed-contexts regex %q: %w", p, err)
			}
		}
	}
	return nil
}

// IsContextAllowed checks whether the given context name passes the
// allow/deny filter. Deny takes precedence over allow.
func (c *Config) IsContextAllowed(name string) bool {
	if matchesAny(name, c.DeniedContexts) {
		return false
	}
	return matchesAny(name, c.AllowedContexts)
}

// KubeconfigPaths returns the list of kubeconfig file paths, splitting on
// the OS path list separator (colon on Unix).
func (c *Config) KubeconfigPaths() []string {
	if c.Kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		return []string{filepath.Join(home, ".kube", "config")}
	}
	return filepath.SplitList(c.Kubeconfig)
}

// matchesAny checks if name matches any of the given patterns.
// Patterns starting and ending with / are treated as regex; others as globs.
func matchesAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if isRegex(p) {
			re, err := regexp.Compile(trimRegex(p))
			if err != nil {
				continue
			}
			if re.MatchString(name) {
				return true
			}
		} else {
			matched, err := filepath.Match(p, name)
			if err != nil {
				continue
			}
			if matched {
				return true
			}
		}
	}
	return false
}

func isRegex(p string) bool {
	return strings.HasPrefix(p, "/") && strings.HasSuffix(p, "/") && len(p) > 2
}

func trimRegex(p string) string {
	return p[1 : len(p)-1]
}
