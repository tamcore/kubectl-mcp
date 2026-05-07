package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidate_Transport(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		wantErr   string
	}{
		{"stdio is valid", "stdio", ""},
		{"sse is valid", "sse", ""},
		{"streamable-http is valid", "streamable-http", ""},
		{"empty is invalid", "", `invalid transport "": must be stdio, sse, or streamable-http`},
		{"unknown is invalid", "grpc", `invalid transport "grpc": must be stdio, sse, or streamable-http`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Transport: tt.transport}
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("got %q, want %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestValidate_AllowWriteNoWarning(t *testing.T) {
	// AllowWrite should no longer emit a warning (write ops are implemented).
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	c := &Config{Transport: "stdio", AllowWrite: true}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = w.Close()
	os.Stderr = origStderr

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "WARNING") {
		t.Fatalf("expected no warning on stderr, got %q", buf.String())
	}
}

func TestValidate_AllowDestructiveImpliesAllowWrite(t *testing.T) {
	c := &Config{Transport: "stdio", AllowDestructive: true}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.AllowWrite {
		t.Fatal("AllowDestructive should imply AllowWrite")
	}
}

func TestValidate_AllowDestructiveWithoutAllowWrite(t *testing.T) {
	c := &Config{Transport: "stdio", AllowDestructive: true, AllowWrite: false}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.AllowWrite {
		t.Fatal("AllowDestructive should force AllowWrite to true")
	}
}

func TestValidate_InvalidDeniedContextsRegex(t *testing.T) {
	c := &Config{
		Transport:      "stdio",
		DeniedContexts: []string{"/[invalid/"},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for invalid denied-contexts regex")
	}
	if !strings.Contains(err.Error(), "invalid denied-contexts regex") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_InvalidAllowedContextsRegex(t *testing.T) {
	c := &Config{
		Transport:       "stdio",
		AllowedContexts: []string{"/[bad/"},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for invalid allowed-contexts regex")
	}
	if !strings.Contains(err.Error(), "invalid allowed-contexts regex") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_RateLimitNegative(t *testing.T) {
	tests := []struct {
		name    string
		read    int
		write   int
		wantErr string
	}{
		{"both zero is valid", 0, 0, ""},
		{"positive values are valid", 120, 30, ""},
		{"negative read is invalid", -1, 30, "rate-limit-read"},
		{"negative write is invalid", 120, -5, "rate-limit-write"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Transport: "stdio", RateLimitRead: tt.read, RateLimitWrite: tt.write}
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestValidate_ValidRegexPatterns(t *testing.T) {
	c := &Config{
		Transport:       "stdio",
		AllowedContexts: []string{"/^prod-.*/"},
		DeniedContexts:  []string{"/^test-.*/"},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_DeniedCheckedBeforeAllowed(t *testing.T) {
	// Invalid denied regex should be caught even if allowed is also invalid.
	c := &Config{
		Transport:       "stdio",
		DeniedContexts:  []string{"/[bad/"},
		AllowedContexts: []string{"/[also-bad/"},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "denied-contexts") {
		t.Fatalf("expected denied-contexts error first, got: %v", err)
	}
}

func TestIsContextAllowed(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		denied  []string
		context string
		want    bool
	}{
		{
			name:    "glob match allowed",
			allowed: []string{"prod-*"},
			context: "prod-us",
			want:    true,
		},
		{
			name:    "regex match allowed",
			allowed: []string{"/^staging-.+$/"},
			context: "staging-eu",
			want:    true,
		},
		{
			name:    "deny takes precedence",
			allowed: []string{"*"},
			denied:  []string{"prod-*"},
			context: "prod-us",
			want:    false,
		},
		{
			name:    "no match returns false",
			allowed: []string{"prod-*"},
			context: "staging-eu",
			want:    false,
		},
		{
			name:    "empty allowed list",
			allowed: nil,
			context: "anything",
			want:    false,
		},
		{
			name:    "empty denied list allows through",
			allowed: []string{"*"},
			denied:  nil,
			context: "anything",
			want:    true,
		},
		{
			name:    "exact match",
			allowed: []string{"my-context"},
			context: "my-context",
			want:    true,
		},
		{
			name:    "denied regex",
			allowed: []string{"*"},
			denied:  []string{"/^prod-.*/"},
			context: "prod-us",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				AllowedContexts: tt.allowed,
				DeniedContexts:  tt.denied,
			}
			if got := c.IsContextAllowed(tt.context); got != tt.want {
				t.Fatalf("IsContextAllowed(%q) = %v, want %v", tt.context, got, tt.want)
			}
		})
	}
}

func TestKubeconfigPaths(t *testing.T) {
	t.Run("empty with no home dir returns nil", func(t *testing.T) {
		home := os.Getenv("HOME")
		_ = os.Unsetenv("HOME")
		t.Cleanup(func() { _ = os.Setenv("HOME", home) })

		c := &Config{}
		paths := c.KubeconfigPaths()
		if paths != nil {
			t.Fatalf("expected nil, got %v", paths)
		}
	})

	t.Run("empty defaults to home kube config", func(t *testing.T) {
		c := &Config{}
		paths := c.KubeconfigPaths()
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("cannot get home dir")
		}
		want := filepath.Join(home, ".kube", "config")
		if len(paths) != 1 || paths[0] != want {
			t.Fatalf("got %v, want [%s]", paths, want)
		}
	})

	t.Run("single path", func(t *testing.T) {
		c := &Config{Kubeconfig: "/tmp/my-kubeconfig"}
		paths := c.KubeconfigPaths()
		if len(paths) != 1 || paths[0] != "/tmp/my-kubeconfig" {
			t.Fatalf("got %v, want [/tmp/my-kubeconfig]", paths)
		}
	})

	t.Run("multiple paths colon separated", func(t *testing.T) {
		c := &Config{Kubeconfig: "/tmp/a:/tmp/b:/tmp/c"}
		paths := c.KubeconfigPaths()
		want := []string{"/tmp/a", "/tmp/b", "/tmp/c"}
		if len(paths) != len(want) {
			t.Fatalf("got %v, want %v", paths, want)
		}
		for i := range want {
			if paths[i] != want[i] {
				t.Fatalf("paths[%d] = %q, want %q", i, paths[i], want[i])
			}
		}
	})
}

func TestMatchesAny(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		patterns []string
		want     bool
	}{
		{"glob star", "prod-us", []string{"prod-*"}, true},
		{"glob question mark", "abc", []string{"a?c"}, true},
		{"glob no match", "staging", []string{"prod-*"}, false},
		{"regex match", "staging-eu", []string{"/^staging-.+$/"}, true},
		{"regex no match", "prod-us", []string{"/^staging-.+$/"}, false},
		{"invalid regex skipped", "anything", []string{"/[invalid/"}, false},
		{"invalid glob skipped", "anything", []string{"[invalid"}, false},
		{"empty patterns", "anything", nil, false},
		{"empty patterns slice", "anything", []string{}, false},
		{"multiple patterns first matches", "dev", []string{"dev", "staging"}, true},
		{"multiple patterns second matches", "staging", []string{"dev", "staging"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesAny(tt.input, tt.patterns); got != tt.want {
				t.Fatalf("matchesAny(%q, %v) = %v, want %v", tt.input, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestValidate_LogLevel(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
		wantErr  string
	}{
		{"off is valid", "off", ""},
		{"info is valid", "info", ""},
		{"debug is valid", "debug", ""},
		{"empty defaults to info", "", ""},
		{"invalid level", "trace", "invalid log level"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Transport: "stdio", LogLevel: tt.logLevel}
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got %q, want error containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestValidate_LogLevel_DefaultsToInfo(t *testing.T) {
	c := &Config{Transport: "stdio"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.LogLevel != "info" {
		t.Fatalf("expected LogLevel to default to 'info', got %q", c.LogLevel)
	}
}

func TestValidate_LogFile_DefaultWhenEmpty(t *testing.T) {
	c := &Config{Transport: "stdio", LogLevel: "info"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.LogFile == "" {
		t.Fatal("expected LogFile to be set to default path, got empty")
	}
	if !strings.Contains(c.LogFile, ".kubectl-mcp") {
		t.Fatalf("expected default LogFile to contain .kubectl-mcp, got %q", c.LogFile)
	}
}

func TestValidate_LogFile_PreservedWhenSet(t *testing.T) {
	c := &Config{Transport: "stdio", LogLevel: "info", LogFile: "/tmp/custom.log"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.LogFile != "/tmp/custom.log" {
		t.Fatalf("expected LogFile to be preserved, got %q", c.LogFile)
	}
}

func TestValidate_LogFile_SkippedWhenOff(t *testing.T) {
	c := &Config{Transport: "stdio", LogLevel: "off"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When log level is off, LogFile doesn't need to be set.
}

func TestValidate_LogDir_DefaultWhenEmpty(t *testing.T) {
	c := &Config{Transport: "stdio", LogLevel: "info"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.LogDir == "" {
		t.Fatal("expected LogDir to be set to default, got empty")
	}
	if !strings.Contains(c.LogDir, ".kubectl-mcp") {
		t.Fatalf("expected default LogDir to contain .kubectl-mcp, got %q", c.LogDir)
	}
}

func TestValidate_LogDir_PreservedWhenSet(t *testing.T) {
	c := &Config{Transport: "stdio", LogLevel: "info", LogDir: "/tmp/custom-logs"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.LogDir != "/tmp/custom-logs" {
		t.Fatalf("expected LogDir to be preserved, got %q", c.LogDir)
	}
}

func TestValidate_LogDir_DerivedFromLogFile(t *testing.T) {
	c := &Config{Transport: "stdio", LogLevel: "info", LogFile: "/tmp/custom/server.log"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.LogDir != "/tmp/custom" {
		t.Fatalf("expected LogDir derived from LogFile parent, got %q", c.LogDir)
	}
}

func TestValidate_LogDir_SkippedWhenOff(t *testing.T) {
	c := &Config{Transport: "stdio", LogLevel: "off"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.LogDir != "" {
		t.Fatalf("expected LogDir to remain empty at off level, got %q", c.LogDir)
	}
}

func TestValidate_LogDir_TakesPrecedenceOverLogFile(t *testing.T) {
	c := &Config{Transport: "stdio", LogLevel: "info", LogDir: "/tmp/explicit-dir", LogFile: "/tmp/other/server.log"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.LogDir != "/tmp/explicit-dir" {
		t.Fatalf("LogDir should take precedence, got %q", c.LogDir)
	}
}

func TestIsRegex(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/^foo$/", true},
		{"/a/", true},
		{"foo", false},
		{"/foo", false},
		{"foo/", false},
		{"", false},
		{"/", false},
		{"//", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isRegex(tt.input); got != tt.want {
				t.Fatalf("isRegex(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTrimRegex(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/^foo$/", "^foo$"},
		{"/abc/", "abc"},
		{"/a/", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := trimRegex(tt.input); got != tt.want {
				t.Fatalf("trimRegex(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidate_SafetyDelay(t *testing.T) {
	tests := []struct {
		name             string
		safetyDelayWrite time.Duration
		safetyDelayDest  time.Duration
		wantErr          string
	}{
		{"both zero is valid", 0, 0, ""},
		{"positive write is valid", 3 * time.Second, 0, ""},
		{"positive destructive is valid", 0, 3 * time.Second, ""},
		{"both positive is valid", 3 * time.Second, 5 * time.Second, ""},
		{"negative write is invalid", -1 * time.Second, 0, "safety-delay-write"},
		{"negative destructive is invalid", 0, -1 * time.Second, "safety-delay-destructive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				Transport:              "stdio",
				SafetyDelayWrite:       tt.safetyDelayWrite,
				SafetyDelayDestructive: tt.safetyDelayDest,
			}
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}
