package kube

import (
	"net/http"
	"sort"
	"testing"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// helper to build a ClientPool with a synthetic rawConfig.
func newTestPool(cfg *config.Config, currentCtx string, ctxNames ...string) *ClientPool {
	contexts := make(map[string]*clientcmdapi.Context, len(ctxNames))
	for _, n := range ctxNames {
		contexts[n] = &clientcmdapi.Context{}
	}
	return &ClientPool{
		cfg: cfg,
		rawConfig: clientcmdapi.Config{
			CurrentContext: currentCtx,
			Contexts:       contexts,
		},
		clients: make(map[string]*ContextClient),
	}
}

// sorted returns a sorted copy of a string slice for deterministic comparison.
func sorted(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}

// --------------- Contexts() ---------------

func TestContexts_AllAllowed(t *testing.T) {
	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"*"}},
		"ctx-a", "ctx-a", "ctx-b", "ctx-c",
	)
	got := sorted(pool.Contexts())
	want := []string{"ctx-a", "ctx-b", "ctx-c"}
	if len(got) != len(want) {
		t.Fatalf("Contexts() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Contexts()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestContexts_SomeDenied(t *testing.T) {
	pool := newTestPool(
		&config.Config{
			AllowedContexts: []string{"*"},
			DeniedContexts:  []string{"ctx-b"},
		},
		"ctx-a", "ctx-a", "ctx-b", "ctx-c",
	)
	got := sorted(pool.Contexts())
	want := []string{"ctx-a", "ctx-c"}
	if len(got) != len(want) {
		t.Fatalf("Contexts() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Contexts()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestContexts_RegexFilter(t *testing.T) {
	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"/^prod-/"}},
		"prod-us", "prod-us", "prod-eu", "staging", "dev",
	)
	got := sorted(pool.Contexts())
	want := []string{"prod-eu", "prod-us"}
	if len(got) != len(want) {
		t.Fatalf("Contexts() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Contexts()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestContexts_NoneAllowed(t *testing.T) {
	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"no-match"}},
		"ctx-a", "ctx-a", "ctx-b",
	)
	got := pool.Contexts()
	if len(got) != 0 {
		t.Fatalf("Contexts() = %v, want empty", got)
	}
}

func TestContexts_DenyTakesPrecedence(t *testing.T) {
	pool := newTestPool(
		&config.Config{
			AllowedContexts: []string{"*"},
			DeniedContexts:  []string{"/^ctx-/"},
		},
		"ctx-a", "ctx-a", "ctx-b", "other",
	)
	got := sorted(pool.Contexts())
	want := []string{"other"}
	if len(got) != len(want) {
		t.Fatalf("Contexts() = %v, want %v", got, want)
	}
	if got[0] != want[0] {
		t.Fatalf("Contexts()[0] = %q, want %q", got[0], want[0])
	}
}

// --------------- DefaultContext() ---------------

func TestDefaultContext_ExplicitConfig(t *testing.T) {
	pool := newTestPool(
		&config.Config{Context: "explicit", AllowedContexts: []string{"*"}},
		"current", "current", "explicit",
	)
	if got := pool.DefaultContext(); got != "explicit" {
		t.Fatalf("DefaultContext() = %q, want %q", got, "explicit")
	}
}

func TestDefaultContext_FallsBackToCurrentContext(t *testing.T) {
	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"*"}},
		"current", "current",
	)
	if got := pool.DefaultContext(); got != "current" {
		t.Fatalf("DefaultContext() = %q, want %q", got, "current")
	}
}

func TestDefaultContext_Empty(t *testing.T) {
	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"*"}},
		"", // no current context
	)
	if got := pool.DefaultContext(); got != "" {
		t.Fatalf("DefaultContext() = %q, want empty", got)
	}
}

// --------------- ResolveContext() ---------------

func TestResolveContext_ValidRequested(t *testing.T) {
	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"*"}},
		"ctx-a", "ctx-a", "ctx-b",
	)
	got, err := pool.ResolveContext("ctx-b")
	if err != nil {
		t.Fatalf("ResolveContext(ctx-b) error: %v", err)
	}
	if got != "ctx-b" {
		t.Fatalf("ResolveContext(ctx-b) = %q, want %q", got, "ctx-b")
	}
}

func TestResolveContext_EmptyFallsBackToDefault(t *testing.T) {
	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"*"}},
		"ctx-a", "ctx-a", "ctx-b",
	)
	got, err := pool.ResolveContext("")
	if err != nil {
		t.Fatalf("ResolveContext('') error: %v", err)
	}
	if got != "ctx-a" {
		t.Fatalf("ResolveContext('') = %q, want %q", got, "ctx-a")
	}
}

func TestResolveContext_EmptyFallsBackToExplicitDefault(t *testing.T) {
	pool := newTestPool(
		&config.Config{Context: "ctx-b", AllowedContexts: []string{"*"}},
		"ctx-a", "ctx-a", "ctx-b",
	)
	got, err := pool.ResolveContext("")
	if err != nil {
		t.Fatalf("ResolveContext('') error: %v", err)
	}
	if got != "ctx-b" {
		t.Fatalf("ResolveContext('') = %q, want %q", got, "ctx-b")
	}
}

func TestResolveContext_DisallowedContext(t *testing.T) {
	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"ctx-a"}},
		"ctx-a", "ctx-a", "ctx-b",
	)
	_, err := pool.ResolveContext("ctx-b")
	if err == nil {
		t.Fatal("ResolveContext(ctx-b) expected error for disallowed context")
	}
}

func TestResolveContext_NonExistentContext(t *testing.T) {
	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"*"}},
		"ctx-a", "ctx-a",
	)
	_, err := pool.ResolveContext("missing")
	if err == nil {
		t.Fatal("ResolveContext(missing) expected error for non-existent context")
	}
}

func TestResolveContext_DeniedContext(t *testing.T) {
	pool := newTestPool(
		&config.Config{
			AllowedContexts: []string{"*"},
			DeniedContexts:  []string{"ctx-b"},
		},
		"ctx-a", "ctx-a", "ctx-b",
	)
	_, err := pool.ResolveContext("ctx-b")
	if err == nil {
		t.Fatal("ResolveContext(ctx-b) expected error for denied context")
	}
}

func TestResolveContext_EmptyDefaultNotInKubeconfig(t *testing.T) {
	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"*"}},
		"ghost", "ctx-a",
	)
	_, err := pool.ResolveContext("")
	if err == nil {
		t.Fatal("ResolveContext('') expected error when default context not in kubeconfig")
	}
}

// --------------- TransportWrapper ---------------

func TestNewClientPool_WithTransportWrapper(t *testing.T) {
	called := false
	wrapper := func(rt http.RoundTripper) http.RoundTripper {
		called = true
		return rt
	}

	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"*"}},
		"ctx-a", "ctx-a",
	)
	pool.transportWrapper = wrapper

	if pool.transportWrapper == nil {
		t.Fatal("expected transportWrapper to be set")
	}

	// Call the wrapper to verify it's the one we set.
	pool.transportWrapper(http.DefaultTransport)
	if !called {
		t.Fatal("expected transportWrapper to be called")
	}
}

func TestNewClientPool_WithoutTransportWrapper(t *testing.T) {
	pool := newTestPool(
		&config.Config{AllowedContexts: []string{"*"}},
		"ctx-a", "ctx-a",
	)

	if pool.transportWrapper != nil {
		t.Fatal("expected transportWrapper to be nil by default")
	}
}
