package cmd

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

var cfg config.Config

var (
	appVersion = "dev"
	appCommit  = "unknown"
)

// SetVersion sets the application version and commit for use in the MCP server.
func SetVersion(version, commit string) {
	appVersion = version
	appCommit = commit
	rootCmd.Version = appVersion + " (" + appCommit + ")"
}

var rootCmd = &cobra.Command{
	Use:   "kubectl-mcp",
	Short: "A read-only Kubernetes MCP server for LLMs",
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfg.Transport, "transport", "stdio", "MCP transport: stdio, sse, or streamable-http")
	rootCmd.PersistentFlags().StringVar(&cfg.SSEAddress, "sse-address", ":8080", "SSE listen address")
	rootCmd.PersistentFlags().StringVar(&cfg.HTTPAddress, "http-address", ":8080", "Streamable-HTTP listen address")
	rootCmd.PersistentFlags().StringVar(&cfg.Kubeconfig, "kubeconfig", "", "Colon-separated kubeconfig paths (defaults to KUBECONFIG env or ~/.kube/config)")
	rootCmd.PersistentFlags().StringVar(&cfg.Context, "context", "", "Default kube-context override")
	rootCmd.PersistentFlags().StringSliceVar(&cfg.AllowedContexts, "allowed-contexts", []string{"*"}, "Comma-separated glob/regex patterns for allowed contexts")
	rootCmd.PersistentFlags().StringSliceVar(&cfg.DeniedContexts, "denied-contexts", nil, "Comma-separated glob/regex patterns for denied contexts")
	rootCmd.PersistentFlags().BoolVar(&cfg.AllowWrite, "allow-write", false, "Enable write operations")
	rootCmd.PersistentFlags().BoolVar(&cfg.AllowDestructive, "allow-destructive", false, "Enable destructive operations (delete, drain); implies --allow-write")
	rootCmd.PersistentFlags().BoolVar(&cfg.AllowSecrets, "allow-secrets", false, "Allow reading Secret data")
	rootCmd.PersistentFlags().BoolVar(&cfg.AllowRaw, "allow-raw", false, "Enable raw Kubernetes API requests (api_raw tool)")
	rootCmd.PersistentFlags().IntVar(&cfg.RateLimitRead, "rate-limit-read", 120, "Max read tool calls per minute (0 = unlimited)")
	rootCmd.PersistentFlags().IntVar(&cfg.RateLimitWrite, "rate-limit-write", 30, "Max write tool calls per minute (0 = unlimited)")
	rootCmd.PersistentFlags().DurationVar(&cfg.SafetyDelayWrite, "safety-delay-write", 3*time.Second, "Pause before write operations (0 to disable)")
	rootCmd.PersistentFlags().DurationVar(&cfg.SafetyDelayDestructive, "safety-delay-destructive", 3*time.Second, "Pause before destructive operations (0 to disable)")
	rootCmd.PersistentFlags().StringVar(&cfg.LogLevel, "log-level", "info", "Logging verbosity: off, info, or debug")
	rootCmd.PersistentFlags().StringVar(&cfg.LogFile, "log-file", "", "Log file path (deprecated: use --log-dir)")
	rootCmd.PersistentFlags().StringVar(&cfg.LogDir, "log-dir", "", "Log directory for per-context log files (default: ~/.kubectl-mcp/)")

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
		cfg.Kubeconfig = os.Getenv("KUBECONFIG")
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
