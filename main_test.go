package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

func TestCheckSymlinkSupport(t *testing.T) {
	err := checkSymlinkSupport()
	if err == nil {
		t.Log("Symlink support check succeeded, indicating symlinks are likely supported.")
		return // Test passes if no error
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "operation not supported") && !strings.Contains(errMsg, "invalid argument") {
			t.Errorf("Symlink support check failed with an unexpected error: %v", err)
	} 
}

func TestRunAutomatic_Symlink(t *testing.T) {
	// This test remains the same, but will be skipped if checkSymlinkSupport() reports an error.
	if err := checkSymlinkSupport(); err != nil {
		t.Skipf("Skipping symlink test as symlinks are likely not supported: %v", err)
		return
	}

	tmpDir := t.TempDir()

	// Create some dummy files
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	if err := os.WriteFile(file1, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run automatic deduplication with symlinks
	preferRegex := regexp.MustCompile("file1")
	err := runAutomatic(false, []string{tmpDir}, preferRegex, nil, false, true)
	if err != nil {
		t.Fatalf("runAutomatic failed: %v", err)
	}

	// Check that file2 is now a symlink to file1
	fi, err := os.Lstat(file2)
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("file2 should be a symlink, but is not")
	}
	linkTarget, err := os.Readlink(file2)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	if linkTarget != file1 {
		t.Errorf("file2 should link to %s, but links to %s", file1, linkTarget)
	}
}
