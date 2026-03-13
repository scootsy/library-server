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

	// Create a second root for multi-root testing
	root2, err := os.MkdirTemp("", "safepath-test2-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root2)

	// Create a symlink for symlink traversal testing
	symlinkDir := filepath.Join(root, "link-to-outside")
	outsideDir, err := os.MkdirTemp("", "safepath-outside-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outsideDir)
	// Symlink creation may fail in some environments; skip those tests if so.
	symlinkCreated := os.Symlink(outsideDir, symlinkDir) == nil

	// Create a deeply nested dir
	deepDir := filepath.Join(subdir, "author", "series", "book")
	if err := os.MkdirAll(deepDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		roots   []string
		wantErr bool
		skip    bool
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
		{
			name:    "traversal attack with ..",
			path:    filepath.Join(subdir, "..", "..", "etc", "passwd"),
			roots:   []string{root},
			wantErr: true,
		},
		{
			name:    "empty string input",
			path:    "",
			roots:   []string{root},
			wantErr: true, // CWD is unlikely to be within root
		},
		{
			name:    "relative path traversal",
			path:    "../../etc/passwd",
			roots:   []string{root},
			wantErr: true,
		},
		{
			name:    "deeply nested valid path",
			path:    deepDir,
			roots:   []string{root},
			wantErr: false,
		},
		{
			name:    "multiple allowed roots - first matches",
			path:    subdir,
			roots:   []string{root, root2},
			wantErr: false,
		},
		{
			name:    "multiple allowed roots - second matches",
			path:    root2,
			roots:   []string{root, root2},
			wantErr: false,
		},
		{
			name:    "non-existent path",
			path:    filepath.Join(root, "nonexistent", "path"),
			roots:   []string{root},
			wantErr: true, // EvalSymlinks fails for non-existent paths
		},
		{
			name:    "symlink traversal to outside root",
			path:    symlinkDir,
			roots:   []string{root},
			wantErr: true,
			skip:    !symlinkCreated,
		},
		{
			name:    "unicode path component",
			path:    filepath.Join(root, "böoks"),
			roots:   []string{root},
			wantErr: true, // doesn't exist
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("symlink creation not supported")
			}
			_, err := SafePath(tt.path, tt.roots...)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSafePathParent(t *testing.T) {
	root, err := os.MkdirTemp("", "safepath-parent-test-*")
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
			name:    "new file in valid directory",
			path:    filepath.Join(subdir, "newfile.json"),
			roots:   []string{root},
			wantErr: false,
		},
		{
			name:    "new file in root directory",
			path:    filepath.Join(root, "metadata.json"),
			roots:   []string{root},
			wantErr: false,
		},
		{
			name:    "new file outside root",
			path:    filepath.Join(os.TempDir(), "evil.json"),
			roots:   []string{subdir},
			wantErr: true,
		},
		{
			name:    "null byte in path",
			path:    filepath.Join(root, "file\x00.json"),
			roots:   []string{root},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafePathParent(tt.path, tt.roots...)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafePathParent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
