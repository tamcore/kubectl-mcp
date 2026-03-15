package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "kubectl-mcp",
	Short: "A read-only Kubernetes MCP server for LLMs",
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
