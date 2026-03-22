package kube

import (
	"fmt"
	"net/http"
	"sync"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// ClientPool manages lazily-created Kubernetes clients for multiple contexts.
type ClientPool struct {
	cfg              *config.Config
	rawConfig        clientcmdapi.Config
	mu               sync.RWMutex
	clients          map[string]*ContextClient
	transportWrapper func(http.RoundTripper) http.RoundTripper
}

// ContextClient bundles the clients needed for a single kube-context.
type ContextClient struct {
	Dynamic    dynamic.Interface
	Clientset  kubernetes.Interface
	Discovery  discovery.DiscoveryInterface
	RestConfig *rest.Config
}

// NewClientPool loads and merges all kubeconfig files and returns a pool.
func NewClientPool(cfg *config.Config) (*ClientPool, error) {
	rules := &clientcmd.ClientConfigLoadingRules{
		Precedence: cfg.KubeconfigPaths(),
	}
	apiConfig, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	return &ClientPool{
		cfg:       cfg,
		rawConfig: *apiConfig,
		clients:   make(map[string]*ContextClient),
	}, nil
}

// SetTransportWrapper sets an HTTP transport wrapper that will be applied to
// all Kubernetes REST clients created by this pool. Must be called before
// any ClientFor calls.
func (p *ClientPool) SetTransportWrapper(wrapper func(http.RoundTripper) http.RoundTripper) {
	p.transportWrapper = wrapper
}

// Contexts returns the list of allowed context names.
func (p *ClientPool) Contexts() []string {
	var names []string
	for name := range p.rawConfig.Contexts {
		if p.cfg.IsContextAllowed(name) {
			names = append(names, name)
		}
	}
	return names
}

// DefaultContext returns the effective default context name.
func (p *ClientPool) DefaultContext() string {
	if p.cfg.Context != "" {
		return p.cfg.Context
	}
	return p.rawConfig.CurrentContext
}

// ResolveContext returns the context to use: the provided name if non-empty
// and allowed, otherwise the default.
func (p *ClientPool) ResolveContext(requested string) (string, error) {
	if requested == "" {
		requested = p.DefaultContext()
	}
	if !p.cfg.IsContextAllowed(requested) {
		return "", fmt.Errorf("context %q is not allowed", requested)
	}
	if _, ok := p.rawConfig.Contexts[requested]; !ok {
		return "", fmt.Errorf("context %q not found in kubeconfig", requested)
	}
	return requested, nil
}

// ClientFor returns (or lazily creates) the clients for the given context name.
func (p *ClientPool) ClientFor(contextName string) (*ContextClient, error) {
	p.mu.RLock()
	if c, ok := p.clients[contextName]; ok {
		p.mu.RUnlock()
		return c, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock.
	if c, ok := p.clients[contextName]; ok {
		return c, nil
	}

	restCfg, err := p.restConfigFor(contextName)
	if err != nil {
		return nil, err
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client for context %q: %w", contextName, err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating clientset for context %q: %w", contextName, err)
	}

	cc := &ContextClient{
		Dynamic:    dynClient,
		Clientset:  clientset,
		Discovery:  clientset.Discovery(),
		RestConfig: restCfg,
	}
	p.clients[contextName] = cc
	return cc, nil
}

func (p *ClientPool) restConfigFor(contextName string) (*rest.Config, error) {
	overrides := &clientcmd.ConfigOverrides{
		CurrentContext: contextName,
	}
	loader := &clientcmd.ClientConfigLoadingRules{
		Precedence: p.cfg.KubeconfigPaths(),
	}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loader, overrides)
	restCfg, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building rest config for context %q: %w", contextName, err)
	}
	if p.transportWrapper != nil {
		restCfg.WrapTransport = p.transportWrapper
	}
	return restCfg, nil
}
