package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

const (
	defaultPortForwardTimeout = 300 * time.Second
	maxPortForwardTimeout     = 3600 * time.Second
)

// PortForwardRequest holds the parameters for a port-forward operation.
type PortForwardRequest struct {
	Namespace  string
	Pod        string
	RemotePort uint16
	LocalPort  uint16
	Timeout    time.Duration
}

// PortForwardResult holds the outcome of a successful port-forward.
type PortForwardResult struct {
	LocalPort uint16
	StopCh    chan struct{}
}

// PortForwarder abstracts the port-forward setup for testability.
type PortForwarder interface {
	Forward(ctx context.Context, clientset kubernetes.Interface, cfg *rest.Config, req PortForwardRequest) (*PortForwardResult, error)
}

// activeForwards tracks live port-forward sessions for cleanup.
var activeForwards sync.Map

// portForwardSession wraps the stop channel with a sync.Once so concurrent
// callers (timeout goroutine and stop_port_forward) can both safely call stop
// without risking a "close of closed channel" panic.
type portForwardSession struct {
	stopCh chan struct{}
	once   sync.Once
}

func (s *portForwardSession) stop() {
	s.once.Do(func() { close(s.stopCh) })
}

// portForwardResponse is the JSON response returned to the caller.
type portForwardResponse struct {
	Pod        string  `json:"pod"`
	Namespace  string  `json:"namespace"`
	LocalPort  uint16  `json:"localPort"`
	RemotePort uint16  `json:"remotePort"`
	Timeout    float64 `json:"timeout"`
	Message    string  `json:"message"`
}

// kindAliases maps normalised kind names and aliases to a canonical form.
var kindAliases = map[string]string{
	"pod":          "pod",
	"pods":         "pod",
	"svc":          "service",
	"service":      "service",
	"services":     "service",
	"deploy":       "deployment",
	"deployment":   "deployment",
	"deployments":  "deployment",
	"sts":          "statefulset",
	"statefulset":  "statefulset",
	"statefulsets": "statefulset",
}

// supportedKinds is the list shown to users in error messages.
var supportedKinds = []string{"Pod", "Service", "Deployment", "StatefulSet"}

// parseResource splits a "kind/name" or bare "name" resource string into
// (canonicalKind, name). Kind is lowercased and alias-expanded.
// Unknown kinds are returned as-is (lowercased).
func parseResource(resource string) (kind, name string) {
	if before, after, ok := strings.Cut(resource, "/"); ok {
		rawKind := strings.ToLower(before)
		name = after
		if canonical, ok := kindAliases[rawKind]; ok {
			return canonical, name
		}
		return rawKind, name
	}
	// Bare name — default to pod.
	return "pod", resource
}

// resolvePortForwardPod returns the name of a ready pod to forward to,
// resolving from a Service, Deployment, or StatefulSet as needed.
// For a "pod" kind it just returns name directly.
func resolvePortForwardPod(ctx context.Context, cc *kube.ContextClient, namespace, kind, name string) (string, error) {
	switch kind {
	case "pod":
		return name, nil

	case "service":
		return resolveFromService(ctx, cc, namespace, name)

	case "deployment":
		return resolveFromDeployment(ctx, cc, namespace, name)

	case "statefulset":
		return resolveFromStatefulSet(ctx, cc, namespace, name)

	default:
		return "", fmt.Errorf("unsupported resource kind %q; supported kinds: %s",
			kind, strings.Join(supportedKinds, ", "))
	}
}

// resolveServiceTargetPort checks whether remotePort matches a named or
// numbered port in the Service and, if so, returns the resolved targetPort.
// If no matching port is found the original remotePort is returned unchanged.
func resolveServiceTargetPort(svc *corev1.Service, remotePort uint16) (uint16, error) {
	if len(svc.Spec.Ports) == 0 {
		return remotePort, nil
	}

	for _, p := range svc.Spec.Ports {
		if uint16(p.Port) == remotePort {
			tp := p.TargetPort
			if tp.Type == 0 { // intstr.Int
				return uint16(tp.IntVal), nil //nolint:gosec
			}
			// Named targetPort — leave resolution to the kubelet; return as-is.
			return remotePort, nil
		}
	}
	return remotePort, nil
}

// resolveFromService finds a ready pod matching a Service's selector.
func resolveFromService(ctx context.Context, cc *kube.ContextClient, namespace, name string) (string, error) {
	svc, err := cc.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting service %q: %w", name, err)
	}

	if len(svc.Spec.Selector) == 0 {
		return "", fmt.Errorf("service %q has no selector; cannot resolve a pod", name)
	}

	selector := labelsToSelector(svc.Spec.Selector)
	return pickReadyPod(ctx, cc, namespace, selector, "")
}

// resolveFromDeployment finds a ready pod matching a Deployment's selector.
func resolveFromDeployment(ctx context.Context, cc *kube.ContextClient, namespace, name string) (string, error) {
	deploy, err := cc.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting deployment %q: %w", name, err)
	}

	if deploy.Spec.Selector == nil || len(deploy.Spec.Selector.MatchLabels) == 0 {
		return "", fmt.Errorf("deployment %q has no selector; cannot resolve a pod", name)
	}

	selector := labelsToSelector(deploy.Spec.Selector.MatchLabels)
	return pickReadyPod(ctx, cc, namespace, selector, "")
}

// resolveFromStatefulSet finds a ready pod matching a StatefulSet's selector,
// preferring the <name>-0 pod.
func resolveFromStatefulSet(ctx context.Context, cc *kube.ContextClient, namespace, name string) (string, error) {
	sts, err := cc.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting statefulset %q: %w", name, err)
	}

	if sts.Spec.Selector == nil || len(sts.Spec.Selector.MatchLabels) == 0 {
		return "", fmt.Errorf("statefulset %q has no selector; cannot resolve a pod", name)
	}

	selector := labelsToSelector(sts.Spec.Selector.MatchLabels)
	preferredPod := name + "-0"
	return pickReadyPod(ctx, cc, namespace, selector, preferredPod)
}

// labelsToSelector converts a label map to a comma-separated k=v selector string.
func labelsToSelector(labels map[string]string) string {
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

// pickReadyPod lists pods matching selector and returns the name of the first
// ready one. If preferredPod is non-empty and ready, it is returned first.
func pickReadyPod(ctx context.Context, cc *kube.ContextClient, namespace, selector, preferredPod string) (string, error) {
	pods, err := cc.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return "", fmt.Errorf("listing pods: %w", err)
	}

	var readyPods []corev1.Pod
	for _, pod := range pods.Items {
		if isPodReady(pod) {
			readyPods = append(readyPods, pod)
		}
	}

	if len(readyPods) == 0 {
		return "", fmt.Errorf("no ready pod found (selector: %s, total pods: %d, phases: %s)",
			selector, len(pods.Items), describePodPhases(pods.Items))
	}

	// Prefer the named pod if it is in the ready list.
	if preferredPod != "" {
		for _, pod := range readyPods {
			if pod.Name == preferredPod {
				return pod.Name, nil
			}
		}
	}

	return readyPods[0].Name, nil
}

// isPodReady returns true when the pod has a Ready condition set to True.
func isPodReady(pod corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// describePodPhases returns a compact summary of pod phases for error messages.
func describePodPhases(pods []corev1.Pod) string {
	if len(pods) == 0 {
		return "<none>"
	}
	counts := make(map[string]int)
	for _, pod := range pods {
		counts[string(pod.Status.Phase)]++
	}
	parts := make([]string, 0, len(counts))
	for phase, n := range counts {
		parts = append(parts, fmt.Sprintf("%s×%d", phase, n))
	}
	return strings.Join(parts, ", ")
}

func registerPortForward(s *server.MCPServer, pool *kube.ClientPool, forwarder PortForwarder, cfg *config.Config) {
	if forwarder == nil {
		forwarder = &spdyPortForwarder{}
	}
	tool := mcp.NewTool("port_forward",
		mcp.WithDescription("Forward a local port to a pod, service, deployment, or statefulset port. Returns the local port number. Requires --allow-write."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace of the target resource"),
		),
		mcp.WithString("resource",
			mcp.Required(),
			mcp.Description(`Target resource in the form "kind/name" or just "name" (defaults to Pod).
Supported kinds: Pod, Service, Deployment, StatefulSet (case-insensitive).
Aliases: svc=Service, deploy=Deployment, sts=StatefulSet.
Examples: "my-pod", "pod/my-pod", "svc/my-service", "deploy/my-app", "sts/my-set"`),
		),
		mcp.WithNumber("remotePort",
			mcp.Required(),
			mcp.Description("Port on the target to forward to"),
		),
		mcp.WithNumber("localPort",
			mcp.Description("Local port to listen on (0 or omit for auto-assign)"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Timeout in seconds (default 300, max 3600). The port-forward will be closed after this duration."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctxName, err := pool.ResolveContext(req.GetString("context", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		cc, err := pool.ClientFor(ctxName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get client: %v", err)), nil
		}

		namespace, _ := req.RequireString("namespace")
		resourceStr, _ := req.RequireString("resource")

		remotePortF := req.GetFloat("remotePort", 0)
		if remotePortF < 1 || remotePortF > 65535 {
			return mcp.NewToolResultError("remotePort must be between 1 and 65535"), nil
		}
		remotePort := uint16(remotePortF)

		localPortF := req.GetFloat("localPort", 0)
		if localPortF < 0 || localPortF > 65535 {
			return mcp.NewToolResultError("localPort must be between 0 and 65535"), nil
		}
		localPort := uint16(localPortF)

		timeout := clampTimeout(req.GetFloat("timeout", defaultPortForwardTimeout.Seconds()))

		kind, name := parseResource(resourceStr)

		// Validate kind before resolution.
		if _, ok := kindAliases[kind]; !ok {
			return mcp.NewToolResultError(fmt.Sprintf(
				"unsupported resource kind %q; supported kinds: %s",
				kind, strings.Join(supportedKinds, ", "),
			)), nil
		}

		// For services: resolve the targetPort if remotePort matches a service port.
		if kind == "service" {
			svc, svcErr := cc.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
			if svcErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("getting service %q: %v", name, svcErr)), nil
			}
			resolvedPort, portErr := resolveServiceTargetPort(svc, remotePort)
			if portErr != nil {
				return mcp.NewToolResultError(portErr.Error()), nil
			}
			remotePort = resolvedPort
		}

		// Resolve to an actual pod name.
		podName, err := resolvePortForwardPod(ctx, cc, namespace, kind, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to resolve pod: %v", err)), nil
		}

		pfReq := PortForwardRequest{
			Namespace:  namespace,
			Pod:        podName,
			RemotePort: remotePort,
			LocalPort:  localPort,
			Timeout:    timeout,
		}

		if err := applySafetyDelay(ctx, req, cfg.SafetyDelayWrite); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("safety delay interrupted: %v", err)), nil
		}

		result, err := forwarder.Forward(ctx, cc.Clientset, cc.RestConfig, pfReq)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to start port forward: %v", err)), nil
		}

		// Store for cleanup and schedule timeout.
		key := fmt.Sprintf("%s/%s/%d", namespace, podName, result.LocalPort)
		session := &portForwardSession{stopCh: result.StopCh}
		activeForwards.Store(key, session)

		go func() {
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case <-timer.C:
				session.stop()
				activeForwards.Delete(key)
			case <-session.stopCh:
				activeForwards.Delete(key)
			}
		}()

		resp := portForwardResponse{
			Pod:        podName,
			Namespace:  namespace,
			LocalPort:  result.LocalPort,
			RemotePort: remotePort,
			Timeout:    timeout.Seconds(),
			Message:    fmt.Sprintf("Port forward active: localhost:%d -> %s:%d (timeout: %s)", result.LocalPort, podName, remotePort, timeout),
		}

		out, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(out)), nil
	})
}

// clampTimeout converts a float seconds value to a clamped Duration.
func clampTimeout(seconds float64) time.Duration {
	timeout := time.Duration(seconds) * time.Second
	if timeout <= 0 {
		timeout = defaultPortForwardTimeout
	}
	if timeout > maxPortForwardTimeout {
		timeout = maxPortForwardTimeout
	}
	return timeout
}
