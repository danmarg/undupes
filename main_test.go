package main

	"bytes"
	"encoding/json"
	"io"
	"log" // Use stdlib log for capturing
	"os"
	"path/filepath"
	"regexp"
	"sort" // Added for sorting names
	"strings"
	"testing"

	dupes "github.com/danmarg/undupes/libdupes" // Import for dupes.Info
	"github.com/stretchr/testify/assert"        // For assertions
	"github.com/stretchr/testify/require"       // For fatal assertions
)

// captureOutput redirects stdout and stderr to a buffer for testing prints/logs.
// It returns the buffer and a function to restore the original streams.
func captureOutput() (*bytes.Buffer, func()) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr
	// Also redirect default logger
	oldLogOut := log.Writer()
	log.SetOutput(wErr) // Redirect stdlib logger to stderr pipe

	outC := make(chan string)
	errC := make(chan string)
	// Copy output in a separate goroutine so printing doesn't block indefinitely
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, rOut)
		outC <- buf.String()
	}()
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, rErr)
		errC <- buf.String()
	}()

	// Return the restore function
	restore := func() {
		wOut.Close()
		wErr.Close()
		os.Stdout = oldStdout // Restore original Stdout
		os.Stderr = oldStderr // Restore original Stderr
		log.SetOutput(oldLogOut) // Restore original log output
	}

	// Read from channels and merge into a single buffer
	var combinedOutput bytes.Buffer
	combinedOutput.WriteString(<-outC)
	combinedOutput.WriteString(<-errC) // Append stderr to stdout for simplicity

	return &combinedOutput, restore
}


// createTestDirWithFiles creates a temporary directory and populates it with files.
// files is a map of relative file path -> file content.
// Returns the temporary directory path and a cleanup function.
func createTestDirWithFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	tmpDir := t.TempDir()
	for name, content := range files {
		filePath := filepath.Join(tmpDir, name)
		// Ensure parent directories exist
		err := os.MkdirAll(filepath.Dir(filePath), 0755)
		require.NoError(t, err, "Failed to create parent directory for %s", name)
		err = os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err, "Failed to write file %s", name)
	}
	return tmpDir
}

// Helper function to create expected dupes.Info for simple cases
func makeExpectedDupes(t *testing.T, tmpDir string, size int64, names ...string) dupes.Info {
	t.Helper()
	absNames := make([]string, len(names))
	for i, name := range names {
		absPath, err := filepath.Abs(filepath.Join(tmpDir, name))
		require.NoError(t, err)
		absNames[i] = absPath
	}
	// Sort names for consistent comparison
	sort.Strings(absNames)
	return dupes.Info{Size: size, Names: absNames}
}

// TODO: TestCreateSymlink - This function seems to be missing from main.go now.
// Remove or update if needed.
/*
func TestCreateSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "source")
	targetPath := filepath.Join(tmpDir, "target")

	// Create dummy source file
	if err := os.WriteFile(sourcePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Call createSymlink
	createSymlink(false, targetPath, sourcePath) // createSymlink doesn't exist

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
*/

// --- Cache Tests ---

func TestCacheCreation(t *testing.T) {
	files := map[string]string{
		"file1.txt": "duplicate content",
		"file2.txt": "duplicate content",
		"unique.txt": "unique content",
	}
	tmpDir := createTestDirWithFiles(t, files)
	cachePath := filepath.Join(tmpDir, "cache.json")
	preferRegex := regexp.MustCompile("file1") // Keep file1

	// Run automatic deduplication with cache enabled
	err := runAutomatic(true, []string{tmpDir}, preferRegex, nil, false, false, cachePath)
	assert.NoError(t, err)

	// Assert cache file exists
	require.FileExists(t, cachePath, "Cache file should be created")

	// Read and verify cache content
	cacheData, err := os.ReadFile(cachePath)
	require.NoError(t, err)

	var cachedDupes []dupes.Info
	err = json.Unmarshal(cacheData, &cachedDupes)
	require.NoError(t, err, "Cache file should contain valid JSON")

	// Use helper for expected dupes
	expectedDupesInfo := makeExpectedDupes(t, tmpDir, int64(len("duplicate content")), "file1.txt", "file2.txt")

	require.Len(t, cachedDupes, 1, "Should find one set of duplicates")
	assert.Equal(t, expectedDupesInfo.Size, cachedDupes[0].Size)
	sort.Strings(cachedDupes[0].Names) // Sort actual names too
	assert.Equal(t, expectedDupesInfo.Names, cachedDupes[0].Names)
}

func TestCacheLoading(t *testing.T) {
	// 1. Setup: Create a temp dir (can be empty for this test)
	tmpDir := createTestDirWithFiles(t, map[string]string{
		// Add some unrelated files to ensure Dupes() isn't run
		"other1.txt": "abc",
		"other2.txt": "def",
	})
	cachePath := filepath.Join(tmpDir, "cache.json")
	preferRegex := regexp.MustCompile(".*") // Doesn't matter as cache should be hit

	// 2. Create a pre-made cache file
	premadeDupes := []dupes.Info{
		makeExpectedDupes(t, tmpDir, 10, "cached/fileA.txt", "cached/fileB.txt"), // Use dummy paths
	}
	premadeData, err := json.MarshalIndent(premadeDupes, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(cachePath, premadeData, 0644)
	require.NoError(t, err, "Failed to write premade cache file")

	// 3. Run with cache flag, capture output
	// Note: We need to run getDupesAndPrintSummary directly or modify runAutomatic
	// to return the dupes list or make logs more verifiable.
	// Let's test getDupesAndPrintSummary directly for easier assertion.
	buf, restore := captureOutput()
	defer restore() // Ensure original streams are restored

	// Call the core function that uses the cache
	foundDupes, err := getDupesAndPrintSummary([]string{tmpDir}, cachePath)
	assert.NoError(t, err)

	// 4. Assertions
	output := buf.String()
	assert.Contains(t, output, "Loaded", "Should print 'Loaded from cache' message")
	assert.NotContains(t, output, "Indexing...", "Should not print 'Indexing...' message when cache is hit")

	// Assert the returned dupes match the pre-made cache content
	require.Len(t, foundDupes, 1)
	assert.Equal(t, premadeDupes[0].Size, foundDupes[0].Size)
	// Paths in premade cache might not be absolute, handle comparison carefully
	// For this test, let's assume makeExpectedDupes created comparable absolute paths
	sort.Strings(foundDupes[0].Names)
	assert.Equal(t, premadeDupes[0].Names, foundDupes[0].Names, "Returned dupes should match cached data")

	// Optional: Assert the cache file was not modified (check mtime or content)
	currentCacheData, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	assert.Equal(t, premadeData, currentCacheData, "Cache file content should not change when loaded")
}

func TestNoCache(t *testing.T) {
	files := map[string]string{
		"f1.txt": "content A",
		"f2.txt": "content B", // Unique file
		"f3.txt": "content A", // Duplicate of f1
	}
	tmpDir := createTestDirWithFiles(t, files)
	cachePath := filepath.Join(tmpDir, "no_cache_used.json") // Define path, but don't use it
	preferRegex := regexp.MustCompile("f1")

	// Capture output to check for "Indexing..."
	buf, restore := captureOutput()
	defer restore()

	// Run automatic without the cachePath argument
	err := runAutomatic(true, []string{tmpDir}, preferRegex, nil, false, false, "") // Empty cachePath
	assert.NoError(t, err)

	// Assertions
	output := buf.String()
	assert.Contains(t, output, "Indexing...", "Should print 'Indexing...' when cache is not used")
	assert.NotContains(t, output, "Loaded", "Should not print 'Loaded from cache'")
	assert.NotContains(t, output, "Saved", "Should not print 'Saved to cache'") // Check save message too

	// Assert cache file does not exist
	assert.NoFileExists(t, cachePath, "Cache file should not be created when --cache flag is absent")
}

func TestInvalidCache(t *testing.T) {
	files := map[string]string{
		"dup1.dat": "same old data",
		"dup2.dat": "same old data",
		"other.dat": "different data",
	}
	tmpDir := createTestDirWithFiles(t, files)
	cachePath := filepath.Join(tmpDir, "invalid_cache.json")
	preferRegex := regexp.MustCompile("dup1")

	// 1. Create an invalid cache file
	invalidContent := []byte("this is not json {")
	err := os.WriteFile(cachePath, invalidContent, 0644)
	require.NoError(t, err, "Failed to write invalid cache file")

	// 2. Capture output
	buf, restore := captureOutput()
	defer restore()

	// 3. Run automatic with the invalid cache path
	err = runAutomatic(true, []string{tmpDir}, preferRegex, nil, false, false, cachePath)
	assert.NoError(t, err) // The function itself shouldn't error, just log warnings

	// 4. Assertions
	output := buf.String()
	// Check for the warning message (adjust based on actual log format)
	// Using Contains might be fragile, depends on the exact log output from main.go
	assert.Contains(t, output, "Failed to unmarshal cache file", "Should log a warning for invalid cache")
	assert.Contains(t, output, "Recomputing duplicates", "Should log a warning for invalid cache")
	assert.Contains(t, output, "Indexing...", "Should print 'Indexing...' after failing to load cache")
	assert.Contains(t, output, "Saved", "Should save the newly computed duplicates") // Verify it saves after recomputing

	// Assert the cache file was overwritten with valid JSON
	require.FileExists(t, cachePath, "Cache file should exist after being overwritten")
	cacheData, err := os.ReadFile(cachePath)
	require.NoError(t, err)

	var cachedDupes []dupes.Info
	err = json.Unmarshal(cacheData, &cachedDupes)
	require.NoError(t, err, "Overwritten cache file should contain valid JSON")

	// Verify the content of the overwritten cache
	expectedDupesInfo := makeExpectedDupes(t, tmpDir, int64(len("same old data")), "dup1.dat", "dup2.dat")
	require.Len(t, cachedDupes, 1, "Should find one set of duplicates after recomputing")
	assert.Equal(t, expectedDupesInfo.Size, cachedDupes[0].Size)
	sort.Strings(cachedDupes[0].Names)
	assert.Equal(t, expectedDupesInfo.Names, cachedDupes[0].Names)
}
