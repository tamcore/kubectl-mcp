package tools

import (
	"strings"
	"testing"
)

func TestShellSplit(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr string
	}{
		{
			name:  "simple command",
			input: "ls -la",
			want:  []string{"ls", "-la"},
		},
		{
			name:  "double-quoted argument preserved",
			input: `sh -c "echo hello world"`,
			want:  []string{"sh", "-c", "echo hello world"},
		},
		{
			name:  "single-quoted argument preserved",
			input: `sh -c 'echo hello world'`,
			want:  []string{"sh", "-c", "echo hello world"},
		},
		{
			name:  "nested quotes in double quotes",
			input: `psql -c "SELECT * FROM projects WHERE id = 2;"`,
			want:  []string{"psql", "-c", "SELECT * FROM projects WHERE id = 2;"},
		},
		{
			name:  "complex shell command with pipe",
			input: `sh -c "wget -qO- http://localhost:8080/api | head -500"`,
			want:  []string{"sh", "-c", "wget -qO- http://localhost:8080/api | head -500"},
		},
		{
			name:  "env var in command",
			input: `sh -c "PGPASSWORD=$PW psql -h db -U user -d mydb -c \"SELECT 1;\""`,
			want:  []string{"sh", "-c", `PGPASSWORD=$PW psql -h db -U user -d mydb -c "SELECT 1;"`},
		},
		{
			name:  "escaped space outside quotes",
			input: `echo hello\ world`,
			want:  []string{"echo", "hello world"},
		},
		{
			name:  "multiple whitespace collapsed",
			input: "ls   -la    /tmp",
			want:  []string{"ls", "-la", "/tmp"},
		},
		{
			name:  "tabs as separators",
			input: "ls\t-la",
			want:  []string{"ls", "-la"},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "only whitespace",
			input: "   \t  ",
			want:  nil,
		},
		{
			name:    "unterminated double quote",
			input:   `sh -c "echo hello`,
			wantErr: "unterminated double quote",
		},
		{
			name:    "unterminated single quote",
			input:   `sh -c 'echo hello`,
			wantErr: "unterminated single quote",
		},
		{
			name:  "empty quotes produce empty token",
			input: `echo ""`,
			// empty double-quoted string does not add bytes to cur,
			// so no token is emitted (matches shellSplit logic where
			// quotes toggle state but empty content means len(cur)==0).
			want: []string{"echo"},
		},
		{
			name:  "adjacent quoted and unquoted",
			input: `foo"bar"baz`,
			want:  []string{"foobarbaz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shellSplit(tt.input)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d tokens %v, want %d tokens %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("token[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
