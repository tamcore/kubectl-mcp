package mcplog

import (
	"log"
	"net/http"
	"regexp"
	"time"
)

// discoveryPrefixes lists K8s API discovery path patterns to skip.
// /api, /apis — root discovery
// /api/v1     — core version discovery (but NOT /api/v1/pods which is a resource)
// /apis/X/v1  — group version discovery (but NOT /apis/apps/v1/deployments)
var coreDiscovery = regexp.MustCompile(`^/api(/[^/]+)?$`)
var groupDiscovery = regexp.MustCompile(`^/apis(/[^/]+(/[^/]+)?)?$`)

// LoggingTransport wraps an http.RoundTripper and logs each HTTP
// request/response to the provided logger. Discovery-only paths are
// filtered out to reduce noise.
type LoggingTransport struct {
	inner  http.RoundTripper
	logger *log.Logger
}

// NewLoggingTransport creates a new LoggingTransport that delegates to inner
// and logs HTTP details to logger.
func NewLoggingTransport(inner http.RoundTripper, logger *log.Logger) *LoggingTransport {
	return &LoggingTransport{inner: inner, logger: logger}
}

// RoundTrip executes the request via the inner transport and logs the result.
func (t *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if isDiscoveryPath(req.URL.Path) {
		return t.inner.RoundTrip(req)
	}

	start := time.Now()
	resp, err := t.inner.RoundTrip(req)
	elapsed := time.Since(start)

	if err != nil {
		t.logger.Printf("  K8s: %s %s → error: %v (%s)", req.Method, req.URL.Path, err, elapsed.Round(time.Millisecond))
		return nil, err
	}

	size := resp.Header.Get("Content-Length")
	if size == "" {
		size = "?"
	}
	t.logger.Printf("  K8s: %s %s → %d (%sB, %s)",
		req.Method, req.URL.Path, resp.StatusCode,
		size, elapsed.Round(time.Millisecond))

	return resp, nil
}

func isDiscoveryPath(path string) bool {
	return coreDiscovery.MatchString(path) || groupDiscovery.MatchString(path)
}
