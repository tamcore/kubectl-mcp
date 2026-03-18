package tools

import (
	"bytes"
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// fakeExecRunner implements ExecRunner for testing.
type fakeExecRunner struct {
	stdout     string
	stderr     string
	execErr    string    // non-empty to simulate an error
	captureCmd *[]string // if set, captures the command slice
}

func (f *fakeExecRunner) Run(_ context.Context, _ kubernetes.Interface, _ *rest.Config, _, _, _ string, command []string, stdout, stderr *bytes.Buffer) error {
	if f.captureCmd != nil {
		*f.captureCmd = append((*f.captureCmd)[:0], command...)
	}
	// Always write stderr so it's available in error responses.
	stderr.WriteString(f.stderr)
	if f.execErr != "" {
		return fmt.Errorf("%s", f.execErr)
	}
	stdout.WriteString(f.stdout)
	return nil
}
