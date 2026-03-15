//go:build envtest

package kube

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// startEnvtest boots an envtest API server and returns a kubeconfig file path
// pointing at it. The caller must call the returned stop function.
func startEnvtest(t *testing.T) (kubeconfigPath string, stop func()) {
	t.Helper()

	env := &envtest.Environment{}
	restCfg, err := env.Start()
	if err != nil {
		t.Fatalf("starting envtest: %v", err)
	}

	dir := t.TempDir()
	kcPath := filepath.Join(dir, "kubeconfig")

	kc := clientcmdapi.Config{
		CurrentContext: "envtest",
		Contexts: map[string]*clientcmdapi.Context{
			"envtest": {Cluster: "envtest", AuthInfo: "envtest"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"envtest": {
				Server:                   restCfg.Host,
				CertificateAuthorityData: restCfg.CAData,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"envtest": {
				ClientCertificateData: restCfg.CertData,
				ClientKeyData:         restCfg.KeyData,
			},
		},
	}

	if err := clientcmd.WriteToFile(kc, kcPath); err != nil {
		_ = env.Stop()
		t.Fatalf("writing kubeconfig: %v", err)
	}

	return kcPath, func() {
		if err := env.Stop(); err != nil {
			t.Logf("stopping envtest: %v", err)
		}
	}
}

func TestNewClientPool_Envtest(t *testing.T) {
	kcPath, stop := startEnvtest(t)
	defer stop()

	// Ensure the default kubeconfig env var doesn't interfere.
	t.Setenv("KUBECONFIG", kcPath)

	cfg := &config.Config{
		Kubeconfig:      kcPath,
		AllowedContexts: []string{"*"},
	}

	pool, err := NewClientPool(cfg)
	if err != nil {
		t.Fatalf("NewClientPool: %v", err)
	}

	ctxs := pool.Contexts()
	if len(ctxs) != 1 || ctxs[0] != "envtest" {
		t.Fatalf("Contexts() = %v, want [envtest]", ctxs)
	}

	if dc := pool.DefaultContext(); dc != "envtest" {
		t.Fatalf("DefaultContext() = %q, want %q", dc, "envtest")
	}
}

func TestClientFor_Envtest(t *testing.T) {
	kcPath, stop := startEnvtest(t)
	defer stop()

	cfg := &config.Config{
		Kubeconfig:      kcPath,
		AllowedContexts: []string{"*"},
	}

	pool, err := NewClientPool(cfg)
	if err != nil {
		t.Fatalf("NewClientPool: %v", err)
	}

	cc, err := pool.ClientFor("envtest")
	if err != nil {
		t.Fatalf("ClientFor(envtest): %v", err)
	}
	if cc == nil {
		t.Fatal("ClientFor returned nil ContextClient")
	}
	if cc.Dynamic == nil {
		t.Fatal("ContextClient.Dynamic is nil")
	}
	if cc.Clientset == nil {
		t.Fatal("ContextClient.Clientset is nil")
	}
	if cc.Discovery == nil {
		t.Fatal("ContextClient.Discovery is nil")
	}
}

func TestClientFor_APIAccess(t *testing.T) {
	kcPath, stop := startEnvtest(t)
	defer stop()

	cfg := &config.Config{
		Kubeconfig:      kcPath,
		AllowedContexts: []string{"*"},
	}

	pool, err := NewClientPool(cfg)
	if err != nil {
		t.Fatalf("NewClientPool: %v", err)
	}

	cc, err := pool.ClientFor("envtest")
	if err != nil {
		t.Fatalf("ClientFor(envtest): %v", err)
	}

	nsList, err := cc.Clientset.CoreV1().Namespaces().List(
		context.Background(), metav1.ListOptions{},
	)
	if err != nil {
		t.Fatalf("listing namespaces: %v", err)
	}
	if len(nsList.Items) == 0 {
		t.Fatal("expected at least one namespace from envtest API server")
	}

	found := false
	for _, ns := range nsList.Items {
		if ns.Name == "default" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'default' namespace in envtest API server")
	}
}

func TestClientFor_Caching(t *testing.T) {
	kcPath, stop := startEnvtest(t)
	defer stop()

	cfg := &config.Config{
		Kubeconfig:      kcPath,
		AllowedContexts: []string{"*"},
	}

	pool, err := NewClientPool(cfg)
	if err != nil {
		t.Fatalf("NewClientPool: %v", err)
	}

	cc1, err := pool.ClientFor("envtest")
	if err != nil {
		t.Fatalf("first ClientFor: %v", err)
	}

	cc2, err := pool.ClientFor("envtest")
	if err != nil {
		t.Fatalf("second ClientFor: %v", err)
	}

	if cc1 != cc2 {
		t.Fatal("ClientFor should return the same cached pointer on repeated calls")
	}
}

func TestClientFor_InvalidContext(t *testing.T) {
	kcPath, stop := startEnvtest(t)
	defer stop()

	cfg := &config.Config{
		Kubeconfig:      kcPath,
		AllowedContexts: []string{"*"},
	}

	pool, err := NewClientPool(cfg)
	if err != nil {
		t.Fatalf("NewClientPool: %v", err)
	}

	_, err = pool.ClientFor("no-such-context")
	if err == nil {
		t.Fatal("ClientFor(no-such-context) should fail")
	}
}

func TestNewClientPool_BadKubeconfig(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad")
	if err := os.WriteFile(bad, []byte("not: valid: kubeconfig: ["), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Kubeconfig:      bad,
		AllowedContexts: []string{"*"},
	}

	_, err := NewClientPool(cfg)
	if err == nil {
		t.Fatal("NewClientPool with broken kubeconfig should fail")
	}
}
