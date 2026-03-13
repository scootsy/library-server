package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafePath(t *testing.T) {
	// Use a real temp directory so EvalSymlinks works
	root, err := os.MkdirTemp("", "safepath-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	subdir := filepath.Join(root, "books")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		roots   []string
		wantErr bool
	}{
		{
			name:    "valid path within root",
			path:    subdir,
			roots:   []string{root},
			wantErr: false,
		},
		{
			name:    "root itself is valid",
			path:    root,
			roots:   []string{root},
			wantErr: false,
		},
		{
			name:    "path outside root",
			path:    os.TempDir(),
			roots:   []string{subdir},
			wantErr: true,
		},
		{
			name:    "null byte in path",
			path:    root + "\x00evil",
			roots:   []string{root},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafePath(tt.path, tt.roots...)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
