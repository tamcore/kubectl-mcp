package main

import (
	"fmt"
	"os"

	"github.com/tamcore/kubectl-mcp/internal/cmd"
)

// Set via ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	cmd.SetVersion(version, commit)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
