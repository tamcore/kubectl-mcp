package kube

import (
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// NewClientPoolForTest creates a ClientPool with pre-loaded clients for testing.
func NewClientPoolForTest(cfg *config.Config, rawConfig clientcmdapi.Config, clients map[string]*ContextClient) *ClientPool {
	return &ClientPool{
		cfg:       cfg,
		rawConfig: rawConfig,
		clients:   clients,
	}
}
