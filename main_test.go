package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "source")
	targetPath := filepath.Join(tmpDir, "target")

	// Create dummy source file
	if err := os.WriteFile(sourcePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Call createSymlink
	createSymlink(false, targetPath, sourcePath)

	// Check if symlink exists
	if _, err := os.Lstat(targetPath); os.IsNotExist(err) {
		t.Errorf("Symlink was not created: %v", err)
	}

	// Check if it's a symlink and the target is correct
	if fi, err := os.Lstat(targetPath); err == nil {
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("Target is not a symlink")
		}

		linkTarget, err := os.Readlink(targetPath)
		if err != nil {
			t.Fatalf("Failed to read symlink target: %v", err)
		}
		if linkTarget != sourcePath {
			t.Errorf("Symlink target is incorrect. Expected: %s, Actual: %s", sourcePath, linkTarget)
		}
	} else {
		t.Errorf("Error checking target file info: %v", err)
	}
}
