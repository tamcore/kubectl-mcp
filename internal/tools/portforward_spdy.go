package tools

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// spdyPortForwarder implements PortForwarder using the SPDY protocol
// via k8s.io/client-go/tools/portforward.
type spdyPortForwarder struct{}

func (s *spdyPortForwarder) Forward(ctx context.Context, clientset kubernetes.Interface, cfg *rest.Config, req PortForwardRequest) (*PortForwardResult, error) {
	url := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(req.Pod).
		Namespace(req.Namespace).
		SubResource("portforward").
		URL()

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating SPDY round tripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)

	portSpec := formatPortSpec(req.LocalPort, req.RemotePort)

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})

	// Capture stderr for diagnostics if the tunnel fails.
	var errBuf bytes.Buffer
	fw, err := portforward.New(dialer, []string{portSpec}, stopCh, readyCh, nil, &errBuf)
	if err != nil {
		return nil, fmt.Errorf("creating port forwarder: %w", err)
	}

	// errCh is buffered so the goroutine can exit even if nobody reads it.
	errCh := make(chan error, 1)
	go func() {
		errCh <- fw.ForwardPorts()
	}()

	select {
	case <-readyCh:
		// Port forward is active — check for immediate failure below.
	case err := <-errCh:
		detail := errBuf.String()
		if detail != "" {
			return nil, fmt.Errorf("port forward failed: %w\n%s", err, detail)
		}
		return nil, fmt.Errorf("port forward failed: %w", err)
	case <-ctx.Done():
		close(stopCh)
		return nil, ctx.Err()
	}

	ports, err := fw.GetPorts()
	if err != nil {
		close(stopCh)
		return nil, fmt.Errorf("getting forwarded ports: %w", err)
	}
	if len(ports) == 0 {
		close(stopCh)
		return nil, fmt.Errorf("no ports returned from port forwarder")
	}

	localPort := ports[0].Local

	// Verify the local listener is actually reachable. On some distributions
	// (e.g., k3s) the SPDY tunnel may die immediately after readyCh fires.
	addr := fmt.Sprintf("127.0.0.1:%d", localPort)
	conn, dialErr := net.DialTimeout("tcp", addr, 2*time.Second)
	if dialErr != nil {
		close(stopCh)
		detail := errBuf.String()
		if detail != "" {
			return nil, fmt.Errorf("port forward listener not reachable at %s: %v\n%s", addr, dialErr, detail)
		}
		return nil, fmt.Errorf("port forward listener not reachable at %s: %v", addr, dialErr)
	}
	_ = conn.Close()

	return &PortForwardResult{
		LocalPort: localPort,
		StopCh:    stopCh,
	}, nil
}

// formatPortSpec builds the "local:remote" port string for client-go.
// A localPort of 0 means the kernel will auto-assign a free port.
func formatPortSpec(localPort, remotePort uint16) string {
	return fmt.Sprintf("%d:%d", localPort, remotePort)
}
