package core

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bazelbuild/bazelisk/config"
	"github.com/bazelbuild/bazelisk/platforms"
)

func TestMaybeDelegateToNoWrapper(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "TestMaybeDelegateToNoWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	os.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())
	expected := "bazel_real"

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}

func TestMaybeDelegateToNoNonExecutableWrapper(t *testing.T) {
	// It's not guaranteed that `tools/bazel` is executable on the
	// Windows host running this test. Thus the test is skipped on
	// this platform to guarantee consistent results.
	if runtime.GOOS == "windows" {
		return
	}

	tmpDir, err := os.MkdirTemp("", "TestMaybeDelegateToNoNonExecutableWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	os.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel"), []byte(""), 0600)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())
	expected := "bazel_real"

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}

func TestMaybeDelegateToStandardWrapper(t *testing.T) {
	// It's not guaranteed that `tools/bazel` is executable on the
	// Windows host running this test. Thus the test is skipped on
	// this platform to guarantee consistent results.
	if runtime.GOOS == "windows" {
		return
	}

	var tmpDir, err = os.MkdirTemp("", "TestMaybeDelegateToStandardWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	os.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel"), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())
	expected := filepath.Join(tmpDir, "tools", "bazel")

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}

func TestMaybeDelegateToPowershellWrapper(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "TestMaybeDelegateToPowershellWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	os.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel.ps1"), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())
	expected := filepath.Join(tmpDir, "tools", "bazel.ps1")

	// Only windows platforms use powershell wrappers
	if runtime.GOOS != "windows" {
		expected = "bazel_real"
	}

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}

func TestMaybeDelegateToBatchWrapper(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "TestMaybeDelegateToBatchWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	os.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel.bat"), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())
	expected := filepath.Join(tmpDir, "tools", "bazel.bat")

	// Only windows platforms use batch wrappers
	if runtime.GOOS != "windows" {
		expected = "bazel_real"
	}

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}

func TestMaybeDelegateToPowershellOverBatchWrapper(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "TestMaybeDelegateToPowershellOverBatchWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	os.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel.ps1"), []byte(""), 0700)
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel.bat"), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())
	expected := filepath.Join(tmpDir, "tools", "bazel.ps1")

	// Only windows platforms use powershell or batch wrappers
	if runtime.GOOS != "windows" {
		expected = "bazel_real"
	}

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}

// Completion Tests

func TestIsCompletionCommand(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "completion bash",
			args:     []string{"completion", "bash"},
			expected: true,
		},
		{
			name:     "completion fish",
			args:     []string{"completion", "fish"},
			expected: true,
		},
		{
			name:     "flags before completion",
			args:     []string{"--some-flag", "completion", "bash"},
			expected: true,
		},
		{
			name:     "not completion command",
			args:     []string{"build", "//..."},
			expected: false,
		},
		{
			name:     "completion not first non-flag",
			args:     []string{"build", "completion"},
			expected: false,
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isCompletionCommand(tc.args)
			if result != tc.expected {
				t.Errorf("isCompletionCommand(%v) = %v, want %v", tc.args, result, tc.expected)
			}
		})
	}
}

func TestConstructInstallerURL(t *testing.T) {
	testCases := []struct {
		name        string
		baseURL     string
		formatURL   string
		version     string
		config      config.Config
		expectError bool
	}{
		{
			name:      "GitHub default",
			baseURL:   "",
			formatURL: "",
			version:   "8.1.1",
			config:    config.Null(),
		},
		{
			name:      "Custom base URL",
			baseURL:   "https://example.com/bazel",
			formatURL: "",
			version:   "8.1.1",
			config:    config.Null(),
		},
		{
			name:        "Both baseURL and formatURL",
			baseURL:     "https://example.com/bazel",
			formatURL:   "https://mirror.com/bazel-%v-installer-%o-%m.sh",
			version:     "8.1.1",
			config:      config.Null(),
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := constructInstallerURL(tc.baseURL, tc.formatURL, tc.version, tc.config)

			if tc.expectError {
				if err == nil {
					t.Errorf("constructInstallerURL() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("constructInstallerURL() unexpected error: %v", err)
				return
			}

			// Dynamically compute expected installer filename based on current platform
			installerFile, err := platforms.DetermineBazelInstallerFilename(tc.version, tc.config)
			if err != nil {
				t.Fatalf("failed to determine installer filename: %v", err)
			}
			var expected string
			if tc.baseURL == "" && tc.formatURL == "" {
				expected = fmt.Sprintf("https://github.com/bazelbuild/bazel/releases/download/%s/%s", tc.version, installerFile)
			} else if tc.baseURL != "" {
				expected = fmt.Sprintf("%s/%s/%s", tc.baseURL, tc.version, installerFile)
			} else {
				// formatURL case is not covered here; skip expectation validation.
			}

			if expected != "" && result != expected {
				t.Errorf("constructInstallerURL() = %v, want %v", result, expected)
			}
		})
	}
}

func TestExtractZipFromInstaller(t *testing.T) {
	// Create a mock installer script with embedded zip
	scriptContent := `#!/bin/bash
echo "This is a bazel installer script"
exit 0
`
	// Create a mock zip file
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	// Add a test file to the zip
	testFile, err := zipWriter.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create test file in zip: %v", err)
	}
	testFile.Write([]byte("test content"))
	zipWriter.Close()

	// Combine script and zip
	installerContent := append([]byte(scriptContent), zipBuffer.Bytes()...)

	zipData, err := extractZipFromInstaller(installerContent)
	if err != nil {
		t.Errorf("extractZipFromInstaller() error: %v", err)
		return
	}

	// Verify the extracted zip data
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Errorf("Could not read extracted zip: %v", err)
		return
	}

	if len(zipReader.File) != 1 {
		t.Errorf("Expected 1 file in zip, got %d", len(zipReader.File))
		return
	}

	if zipReader.File[0].Name != "test.txt" {
		t.Errorf("Expected file name 'test.txt', got '%s'", zipReader.File[0].Name)
	}
}

func TestExtractCompletionScriptsFromZip(t *testing.T) {
	// Create a mock zip file with completion scripts
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	// Add bash completion script
	bashFile, err := zipWriter.Create("bazel-complete.bash")
	if err != nil {
		t.Fatalf("Failed to create bash completion file: %v", err)
	}
	bashContent := "# Bash completion for bazel"
	bashFile.Write([]byte(bashContent))

	// Add fish completion script
	fishFile, err := zipWriter.Create("bazel.fish")
	if err != nil {
		t.Fatalf("Failed to create fish completion file: %v", err)
	}
	fishContent := "# Fish completion for bazel"
	fishFile.Write([]byte(fishContent))

	// Add unrelated file
	otherFile, err := zipWriter.Create("bazel")
	if err != nil {
		t.Fatalf("Failed to create other file: %v", err)
	}
	otherFile.Write([]byte("bazel binary"))

	zipWriter.Close()

	scripts, err := extractCompletionScriptsFromZip(zipBuffer.Bytes())
	if err != nil {
		t.Errorf("extractCompletionScriptsFromZip() error: %v", err)
		return
	}

	if len(scripts) != 2 {
		t.Errorf("Expected 2 completion scripts, got %d", len(scripts))
		return
	}

	if scripts["bazel-complete.bash"] != bashContent {
		t.Errorf("Bash completion content mismatch. Got: %q, want: %q", scripts["bazel-complete.bash"], bashContent)
	}

	if scripts["bazel.fish"] != fishContent {
		t.Errorf("Fish completion content mismatch. Got: %q, want: %q", scripts["bazel.fish"], fishContent)
	}
}

func TestExtractCompletionScriptsFromZipMissingBash(t *testing.T) {
	// Create a zip file without bash completion
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	// Add only fish completion script
	fishFile, err := zipWriter.Create("bazel.fish")
	if err != nil {
		t.Fatalf("Failed to create fish completion file: %v", err)
	}
	fishFile.Write([]byte("# Fish completion for bazel"))

	zipWriter.Close()

	_, err = extractCompletionScriptsFromZip(zipBuffer.Bytes())
	if err == nil {
		t.Error("extractCompletionScriptsFromZip() expected error for missing bash completion but got none")
	}

	expectedError := "bazel-complete.bash not found in zip file"
	if err.Error() != expectedError {
		t.Errorf("extractCompletionScriptsFromZip() error = %q, want %q", err.Error(), expectedError)
	}
}

func TestExtractCompletionScriptsFromZipMissingFish(t *testing.T) {
	// Create a zip file with only bash completion (fish is optional)
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	// Add only bash completion script
	bashFile, err := zipWriter.Create("bazel-complete.bash")
	if err != nil {
		t.Fatalf("Failed to create bash completion file: %v", err)
	}
	bashContent := "# Bash completion for bazel"
	bashFile.Write([]byte(bashContent))

	zipWriter.Close()

	scripts, err := extractCompletionScriptsFromZip(zipBuffer.Bytes())
	if err != nil {
		t.Errorf("extractCompletionScriptsFromZip() unexpected error: %v", err)
		return
	}

	if len(scripts) != 1 {
		t.Errorf("Expected 1 completion script, got %d", len(scripts))
		return
	}

	if scripts["bazel-complete.bash"] != bashContent {
		t.Errorf("Bash completion content mismatch. Got: %q, want: %q", scripts["bazel-complete.bash"], bashContent)
	}

	// Fish completion should not be present
	if _, found := scripts["bazel.fish"]; found {
		t.Error("Fish completion should not be present when not in zip file")
	}
}

func TestHandleCompletionCommandUnsupportedShell(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "TestHandleCompletionCommand")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	installation := &BazelInstallation{
		Version: "8.1.1",
		Path:    "/fake/path/to/bazel",
	}

	args := []string{"completion", "zsh"}
	err = handleCompletionCommand(args, installation, config.Null())

	if err == nil {
		t.Error("handleCompletionCommand() expected error for unsupported shell but got none")
		return
	}

	expectedError := "only bash and fish completion are supported, got: zsh"
	if err.Error() != expectedError {
		t.Errorf("handleCompletionCommand() error = %q, want %q", err.Error(), expectedError)
	}
}

func TestHandleCompletionCommandMissingShell(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "TestHandleCompletionCommand")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	installation := &BazelInstallation{
		Version: "8.1.1",
		Path:    "/fake/path/to/bazel",
	}

	args := []string{"completion"}
	err = handleCompletionCommand(args, installation, config.Null())

	if err == nil {
		t.Error("handleCompletionCommand() expected error for missing shell but got none")
		return
	}

	expectedError := "only bash and fish completion are supported, got: "
	if err.Error() != expectedError {
		t.Errorf("handleCompletionCommand() error = %q, want %q", err.Error(), expectedError)
	}
}

func TestGetBazelCompletionScriptUnsupportedShell(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "TestGetBazelCompletionScript")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = getBazelCompletionScript("8.1.1", tmpDir, "powershell", config.Null())

	if err == nil {
		t.Error("getBazelCompletionScript() expected error for unsupported shell but got none")
		return
	}

	expectedError := "unsupported shell: powershell"
	if err.Error() != expectedError {
		t.Errorf("getBazelCompletionScript() error = %q, want %q", err.Error(), expectedError)
	}
}

func TestCompletionScriptCaching(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "TestCompletionScriptCaching")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock installer content and compute its hash
	installerContent := []byte(`#!/bin/bash
echo "Mock bazel installer"
# Mock installer content`)

	h := sha256.New()
	h.Write(installerContent)
	installerHash := strings.ToLower(fmt.Sprintf("%x", h.Sum(nil)))

	// Create cache directory structure using installer content hash
	casDir := filepath.Join(tmpDir, "downloads", "sha256")
	completionDir := filepath.Join(casDir, installerHash, "completion")
	err = os.MkdirAll(completionDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create completion dir: %v", err)
	}

	// Create metadata mapping
	installerFile, err := platforms.DetermineBazelInstallerFilename("8.1.1", config.Null())
	if err != nil {
		t.Fatalf("failed to determine installer filename: %v", err)
	}
	metadataDir := filepath.Join(tmpDir, "downloads", "metadata", "bazelbuild")
	err = os.MkdirAll(metadataDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create metadata dir: %v", err)
	}
	mappingPath := filepath.Join(metadataDir, installerFile)
	err = os.WriteFile(mappingPath, []byte(installerHash), 0644)
	if err != nil {
		t.Fatalf("Failed to write mapping file: %v", err)
	}

	// Write cached bash completion script
	bashContent := "# Cached bash completion for bazel"
	bashPath := filepath.Join(completionDir, "bazel-complete.bash")
	err = os.WriteFile(bashPath, []byte(bashContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write cached bash completion: %v", err)
	}

	// Write cached fish completion script
	fishContent := "# Cached fish completion for bazel"
	fishPath := filepath.Join(completionDir, "bazel.fish")
	err = os.WriteFile(fishPath, []byte(fishContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write cached fish completion: %v", err)
	}

	// Test reading cached bash completion
	bashResult, err := getBazelCompletionScript("8.1.1", tmpDir, "bash", config.Null())
	if err != nil {
		t.Errorf("getBazelCompletionScript(bash) error: %v", err)
		return
	}

	if bashResult != bashContent {
		t.Errorf("getBazelCompletionScript(bash) = %q, want %q", bashResult, bashContent)
	}

	// Test reading cached fish completion
	fishResult, err := getBazelCompletionScript("8.1.1", tmpDir, "fish", config.Null())
	if err != nil {
		t.Errorf("getBazelCompletionScript(fish) error: %v", err)
		return
	}

	if fishResult != fishContent {
		t.Errorf("getBazelCompletionScript(fish) = %q, want %q", fishResult, fishContent)
	}
}

func TestExtractCompletionScriptsFromInstallerIntegration(t *testing.T) {
	// Create a complete mock installer with embedded zip containing completion scripts
	scriptContent := `#!/bin/bash
echo "Bazel installer script"
# Script content here
exit 0
`

	// Create a mock zip file with completion scripts
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	// Add bash completion script
	bashFile, err := zipWriter.Create("bazel-complete.bash")
	if err != nil {
		t.Fatalf("Failed to create bash completion file: %v", err)
	}
	bashContent := "# Bash completion for bazel\ncomplete -W 'build test run' bazel"
	bashFile.Write([]byte(bashContent))

	// Add fish completion script
	fishFile, err := zipWriter.Create("bazel.fish")
	if err != nil {
		t.Fatalf("Failed to create fish completion file: %v", err)
	}
	fishContent := "# Fish completion for bazel\ncomplete -c bazel -a 'build test run'"
	fishFile.Write([]byte(fishContent))

	// Add bazel binary (mock)
	bazelFile, err := zipWriter.Create("bazel")
	if err != nil {
		t.Fatalf("Failed to create bazel file: %v", err)
	}
	bazelFile.Write([]byte("fake bazel binary"))

	zipWriter.Close()

	// Combine script and zip to create installer
	installerContent := append([]byte(scriptContent), zipBuffer.Bytes()...)

	// Test the complete extraction process
	scripts, err := extractCompletionScriptsFromInstaller(installerContent)
	if err != nil {
		t.Errorf("extractCompletionScriptsFromInstaller() error: %v", err)
		return
	}

	if len(scripts) != 2 {
		t.Errorf("Expected 2 completion scripts, got %d", len(scripts))
		return
	}

	if scripts["bazel-complete.bash"] != bashContent {
		t.Errorf("Bash completion content mismatch")
	}

	if scripts["bazel.fish"] != fishContent {
		t.Errorf("Fish completion content mismatch")
	}
}
