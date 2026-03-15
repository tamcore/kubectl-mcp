package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

var cfg config.Config

var rootCmd = &cobra.Command{
	Use:   "kubectl-mcp",
	Short: "A read-only Kubernetes MCP server for LLMs",
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfg.Transport, "transport", "stdio", "MCP transport: stdio or sse")
	rootCmd.PersistentFlags().StringVar(&cfg.SSEAddress, "sse-address", ":8080", "SSE listen address")
	rootCmd.PersistentFlags().StringVar(&cfg.Kubeconfig, "kubeconfig", "", "Colon-separated kubeconfig paths (defaults to KUBECONFIG env or ~/.kube/config)")
	rootCmd.PersistentFlags().StringVar(&cfg.Context, "context", "", "Default kube-context override")
	rootCmd.PersistentFlags().StringSliceVar(&cfg.AllowedContexts, "allowed-contexts", []string{"*"}, "Comma-separated glob/regex patterns for allowed contexts")
	rootCmd.PersistentFlags().StringSliceVar(&cfg.DeniedContexts, "denied-contexts", nil, "Comma-separated glob/regex patterns for denied contexts")
	rootCmd.PersistentFlags().BoolVar(&cfg.AllowWrite, "allow-write", false, "Enable write operations (reserved for future use)")
	rootCmd.PersistentFlags().BoolVar(&cfg.AllowSecrets, "allow-secrets", false, "Allow reading Secret data")

	rootCmd.AddCommand(serveCmd)
}

func initConfig() {
	v := viper.GetViper()
	v.SetEnvPrefix("KUBECTL_MCP")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	// Bind all persistent flags to viper.
	_ = v.BindPFlags(rootCmd.PersistentFlags())

	// KUBECONFIG is a special case: honour the standard env var directly.
	if cfg.Kubeconfig == "" {
		cfg.Kubeconfig = v.GetString("kubeconfig")
	}
	if cfg.Kubeconfig == "" {
		cfg.Kubeconfig = viper.GetString("KUBECONFIG")
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
