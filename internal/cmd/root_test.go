package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestExecute_Help(t *testing.T) {
	rootCmd.SetArgs([]string{"--help"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	if err := Execute(); err != nil {
		t.Fatalf("Execute(--help) returned error: %v", err)
	}
}

func TestInitConfig_KubeconfigFromEnv(t *testing.T) {
	orig := cfg.Kubeconfig
	t.Cleanup(func() { cfg.Kubeconfig = orig })

	cfg.Kubeconfig = ""
	t.Setenv("KUBECONFIG", "/tmp/test-kubeconfig")

	initConfig()

	if cfg.Kubeconfig != "/tmp/test-kubeconfig" {
		t.Fatalf("cfg.Kubeconfig = %q, want /tmp/test-kubeconfig", cfg.Kubeconfig)
	}
}

func TestInitConfig_FlagTakesPrecedence(t *testing.T) {
	orig := cfg.Kubeconfig
	t.Cleanup(func() { cfg.Kubeconfig = orig })

	cfg.Kubeconfig = "/flag/path"
	t.Setenv("KUBECONFIG", "/env/path")

	initConfig()

	if cfg.Kubeconfig != "/flag/path" {
		t.Fatalf("cfg.Kubeconfig = %q, want /flag/path", cfg.Kubeconfig)
	}
}

func TestServeCmd_ValidationError(t *testing.T) {
	origTransport := cfg.Transport
	t.Cleanup(func() {
		cfg.Transport = origTransport
		rootCmd.SetArgs(nil)
	})

	rootCmd.SetArgs([]string{"serve", "--transport", "invalid"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid transport")
	}
}

func TestServeCmd_KubeClientPoolError(t *testing.T) {
	origTransport := cfg.Transport
	origKubeconfig := cfg.Kubeconfig
	t.Cleanup(func() {
		cfg.Transport = origTransport
		cfg.Kubeconfig = origKubeconfig
		rootCmd.SetArgs(nil)
	})

	// Create a malformed kubeconfig file to trigger a load error.
	tmp := t.TempDir()
	bad := tmp + "/kubeconfig"
	if err := os.WriteFile(bad, []byte("\t\tbad-yaml: [[["), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg.Kubeconfig = bad
	rootCmd.SetArgs([]string{"serve", "--transport", "stdio"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for malformed kubeconfig")
	}
}

func TestRootCmd_HasServeSubcommand(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "serve" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("root command missing 'serve' subcommand")
	}
}

func TestServerInstructions_Default(t *testing.T) {
	instructions := serverInstructions(&config.Config{})
	if strings.Contains(instructions, "Context Required") {
		t.Error("instructions should not mention Context Required by default")
	}
}

func TestServerInstructions_RequireContext(t *testing.T) {
	instructions := serverInstructions(&config.Config{RequireContext: true})
	if !strings.Contains(instructions, "Context Required") {
		t.Error("instructions should mention Context Required when flag is set")
	}
	if !strings.Contains(instructions, "list_contexts") {
		t.Error("instructions should mention list_contexts for discovery")
	}
}
