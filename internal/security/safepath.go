package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafePath resolves a user-influenced path and validates it falls within
// one of the allowed root directories. Returns the cleaned absolute path
// or an error if validation fails.
func SafePath(userPath string, allowedRoots ...string) (string, error) {
	// Strip null bytes
	if strings.ContainsRune(userPath, 0) {
		return "", fmt.Errorf("path contains null byte")
	}

	// Resolve to absolute, following symlinks
	resolved, err := filepath.Abs(userPath)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", fmt.Errorf("evaluating symlinks: %w", err)
	}

	// Check against allowed roots
	for _, root := range allowedRoots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absRoot, err = filepath.EvalSymlinks(absRoot)
		if err != nil {
			continue
		}
		// Ensure the resolved path is within or equal to the root
		if strings.HasPrefix(resolved, absRoot+string(filepath.Separator)) || resolved == absRoot {
			return resolved, nil
		}
	}

	return "", fmt.Errorf("path %q is outside allowed roots", userPath)
}

// SafePathParent validates that the parent directory of userPath falls within
// one of the allowed roots. This is useful for paths that don't exist yet
// (e.g., new files to be created), where EvalSymlinks would fail.
// Returns the cleaned absolute path or an error if validation fails.
func SafePathParent(userPath string, allowedRoots ...string) (string, error) {
	if strings.ContainsRune(userPath, 0) {
		return "", fmt.Errorf("path contains null byte")
	}

	resolved, err := filepath.Abs(userPath)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	// Validate the parent directory (which should exist)
	parentDir := filepath.Dir(resolved)
	parentResolved, err := filepath.EvalSymlinks(parentDir)
	if err != nil {
		return "", fmt.Errorf("evaluating parent symlinks: %w", err)
	}

	for _, root := range allowedRoots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absRoot, err = filepath.EvalSymlinks(absRoot)
		if err != nil {
			continue
		}
		if strings.HasPrefix(parentResolved, absRoot+string(filepath.Separator)) || parentResolved == absRoot {
			return filepath.Join(parentResolved, filepath.Base(resolved)), nil
		}
	}

	return "", fmt.Errorf("path %q is outside allowed roots", userPath)
}
