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
	stdout  string
	stderr  string
	execErr string // non-empty to simulate an error
}

func (f *fakeExecRunner) Run(_ context.Context, _ kubernetes.Interface, _ *rest.Config, _, _, _ string, _ []string, stdout, stderr *bytes.Buffer) error {
	if f.execErr != "" {
		return fmt.Errorf("%s", f.execErr)
	}
	stdout.WriteString(f.stdout)
	stderr.WriteString(f.stderr)
	return nil
}
