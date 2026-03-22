package mcplog

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
)

func TestLoggingTransport_LogsRequestAndResponse(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"kind":"Pod"}`)),
			Header:     http.Header{"Content-Length": []string{"14"}},
		}, nil
	})

	transport := NewLoggingTransport(inner, logger)

	req, _ := http.NewRequest("GET", "https://k8s.example.com/api/v1/namespaces/default/pods/nginx", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	output := buf.String()
	if !strings.Contains(output, "GET") {
		t.Errorf("expected log to contain HTTP method GET, got: %s", output)
	}
	if !strings.Contains(output, "/api/v1/namespaces/default/pods/nginx") {
		t.Errorf("expected log to contain URL path, got: %s", output)
	}
	if !strings.Contains(output, "200") {
		t.Errorf("expected log to contain status code 200, got: %s", output)
	}
}

func TestLoggingTransport_FiltersDiscoveryPaths(t *testing.T) {
	discoveryPaths := []string{
		"https://k8s.example.com/api",
		"https://k8s.example.com/api?timeout=10s",
		"https://k8s.example.com/apis",
		"https://k8s.example.com/apis?timeout=10s",
		"https://k8s.example.com/api/v1",
		"https://k8s.example.com/apis/apps/v1",
	}

	for _, urlStr := range discoveryPaths {
		t.Run(urlStr, func(t *testing.T) {
			var buf bytes.Buffer
			logger := log.New(&buf, "", 0)

			inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			})

			transport := NewLoggingTransport(inner, logger)
			req, _ := http.NewRequest("GET", urlStr, nil)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_ = resp.Body.Close()

			if buf.Len() > 0 {
				t.Errorf("expected no log output for discovery path %s, got: %s", urlStr, buf.String())
			}
		})
	}
}

func TestLoggingTransport_LogsNonDiscoveryPaths(t *testing.T) {
	nonDiscoveryPaths := []string{
		"https://k8s.example.com/api/v1/namespaces",
		"https://k8s.example.com/api/v1/pods",
		"https://k8s.example.com/apis/apps/v1/deployments",
		"https://k8s.example.com/apis/apps/v1/namespaces/default/deployments/nginx",
	}

	for _, urlStr := range nonDiscoveryPaths {
		t.Run(urlStr, func(t *testing.T) {
			var buf bytes.Buffer
			logger := log.New(&buf, "", 0)

			inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{}`)),
					Header:     http.Header{"Content-Length": []string{"2"}},
				}, nil
			})

			transport := NewLoggingTransport(inner, logger)
			req, _ := http.NewRequest("GET", urlStr, nil)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_ = resp.Body.Close()

			if buf.Len() == 0 {
				t.Errorf("expected log output for non-discovery path %s", urlStr)
			}
		})
	}
}

func TestLoggingTransport_HandlesTransportError(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})

	transport := NewLoggingTransport(inner, logger)
	req, _ := http.NewRequest("GET", "https://k8s.example.com/api/v1/namespaces/default/pods", nil)
	_, err := transport.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	output := buf.String()
	if !strings.Contains(output, "error") || !strings.Contains(output, "GET") {
		t.Errorf("expected log to contain error info, got: %s", output)
	}
}

func TestLoggingTransport_DelegatesUnmodified(t *testing.T) {
	expectedBody := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"nginx"}}`
	inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(expectedBody)),
			Header:     make(http.Header),
		}, nil
	})

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	transport := NewLoggingTransport(inner, logger)

	req, _ := http.NewRequest("GET", "https://k8s.example.com/api/v1/namespaces/default/pods/nginx", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if string(body) != expectedBody {
		t.Fatalf("response body modified: got %q, want %q", string(body), expectedBody)
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
