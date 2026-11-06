package core

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bazelbuild/bazelisk/config"
	"github.com/bazelbuild/bazelisk/platforms"
)

func TestDownloadBazelIfNecessaryVerifiesSHA256(t *testing.T) {
	content := []byte("test Bazel binary\n")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	const version = "9.0.0"
	osName, err := platforms.DetermineOperatingSystem()
	if err != nil {
		t.Fatal(err)
	}
	arch, err := platforms.DetermineArchitecture(osName, version)
	if err != nil {
		t.Fatal(err)
	}
	platformKey := "BAZELISK_VERIFY_SHA256_" + strings.ToUpper(osName+"_"+arch)
	noJDKKey := "BAZELISK_VERIFY_SHA256_NOJDK_" + strings.ToUpper(osName+"_"+arch)
	wrongHash := strings.Repeat("0", 64)
	for _, tc := range []struct {
		name     string
		cached   bool
		expected string
		wantErr  bool
		values   map[string]string
	}{
		{name: "cached without expectation", cached: true},
		{name: "cached matching hash", cached: true, expected: digest},
		{name: "cached uppercase hash", cached: true, expected: strings.ToUpper(digest)},
		{name: "cached wrong hash", cached: true, expected: strings.Repeat("0", 64), wantErr: true},
		{name: "cached invalid hash", cached: true, expected: "wrong-hash", wantErr: true},
		{name: "download without expectation"},
		{name: "download matching hash", expected: digest},
		{name: "download uppercase hash", expected: strings.ToUpper(digest)},
		{name: "download wrong hash", expected: strings.Repeat("0", 64), wantErr: true},
		{name: "download invalid hash", expected: "wrong-hash", wantErr: true},
		{name: "download platform hash", values: map[string]string{platformKey: strings.ToUpper(digest)}},
		{name: "platform overrides wrong generic", values: map[string]string{platformKey: digest, "BAZELISK_VERIFY_SHA256": wrongHash}},
		{name: "download wrong platform hash", expected: wrongHash, wantErr: true, values: map[string]string{platformKey: wrongHash, "BAZELISK_VERIFY_SHA256": digest}},
		{name: "cached wrong platform hash", cached: true, expected: wrongHash, wantErr: true, values: map[string]string{platformKey: wrongHash, "BAZELISK_VERIFY_SHA256": digest}},
		{name: "download NOJDK hash", values: map[string]string{"BAZELISK_NOJDK": "1", noJDKKey: strings.ToUpper(digest), platformKey: wrongHash}},
		{name: "download wrong NOJDK hash", expected: wrongHash, wantErr: true, values: map[string]string{"BAZELISK_NOJDK": "1", noJDKKey: wrongHash, platformKey: digest}},
		{name: "cached wrong NOJDK hash", cached: true, expected: wrongHash, wantErr: true, values: map[string]string{"BAZELISK_NOJDK": "1", noJDKKey: wrongHash, platformKey: digest}},
		{name: "false string enables NOJDK", values: map[string]string{"BAZELISK_NOJDK": "false", noJDKKey: digest, platformKey: wrongHash}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			downloads := 0
			downloader := func(destDir, destFile string) (string, error) {
				downloads++
				if err := os.MkdirAll(destDir, 0755); err != nil {
					return "", err
				}
				path := filepath.Join(destDir, destFile)
				return path, os.WriteFile(path, content, 0600)
			}
			repos := CreateRepositories(nil, nil, nil, nil, false)
			values := tc.values
			if values == nil {
				values = map[string]string{"BAZELISK_VERIFY_SHA256": tc.expected}
			}
			cfg := config.Static(values)
			if tc.cached {
				warmConfig := config.Static(map[string]string{"BAZELISK_NOJDK": values["BAZELISK_NOJDK"]})
				if _, err := downloadBazelIfNecessary(version, home, "bazelbuild", repos, warmConfig, downloader); err != nil {
					t.Fatal(err)
				}
			}
			downloads = 0
			path, err := downloadBazelIfNecessary(version, home, "bazelbuild", repos, cfg, downloader)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "need sha256="+tc.expected) {
					t.Fatalf("expected hash mismatch, got path %q, error %v", path, err)
				}
				if path != "" {
					t.Errorf("returned path %q after hash mismatch", path)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			wantDownloads := 1
			if tc.cached {
				wantDownloads = 0
			}
			if downloads != wantDownloads {
				t.Errorf("downloaded %d times, want %d", downloads, wantDownloads)
			}
			filename, err := platforms.DetermineBazelFilename(version, false, cfg)
			if err != nil {
				t.Fatal(err)
			}
			mappingPath := filepath.Join(home, "downloads", "metadata", "bazelbuild", filename)
			mapping, err := os.ReadFile(mappingPath)
			if tc.wantErr && !tc.cached {
				if !os.IsNotExist(err) {
					t.Errorf("failed download created mapping: %q, error %v", mapping, err)
				}
			} else if err != nil || string(mapping) != digest {
				t.Errorf("mapping = %q, error %v, want %q", mapping, err, digest)
			}
			cachedPath := filepath.Join(home, "downloads", "sha256", digest, "bin", "bazel"+platforms.DetermineExecutableFilenameSuffix())
			if !tc.wantErr && path != cachedPath {
				t.Errorf("returned path %q, want %q", path, cachedPath)
			}
			got, err := os.ReadFile(cachedPath)
			if err != nil || !bytes.Equal(got, content) {
				t.Errorf("cached binary = %q, error %v, want %q", got, err, content)
			}
		})
	}
}

func TestGetBazelVersionFromFileInParentDirectory(t *testing.T) {
	root := t.TempDir()
	workingDirectory := filepath.Join(root, "nested", "directory")
	if err := os.MkdirAll(workingDirectory, 0700); err != nil {
		t.Fatalf("Failed to create working directory: %v", err)
	}
	want := "8.5.0"
	if err := os.WriteFile(filepath.Join(root, ".bazelversion"), []byte(want), 0600); err != nil {
		t.Fatalf("Failed to write .bazelversion: %v", err)
	}
	t.Chdir(workingDirectory)

	got, err := GetBazelVersion(config.Null())
	if err != nil {
		t.Fatalf("GetBazelVersion failed unexpectedly: %v", err)
	}
	if got != want {
		t.Fatalf("GetBazelVersion returned %q, want %q", got, want)
	}
}

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

func TestMaybeDelegateToOsAndArchSpecificWrapper(t *testing.T) {
	var tmpDir, err = os.MkdirTemp("", "TestMaybeDelegateToOsAndArchSpecificWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	osName, err := platforms.DetermineOperatingSystem()
	if err != nil {
		log.Fatal(err)
	}
	arch, err := platforms.DetermineArchitecture(osName, platforms.DarwinArm64MinVersion)
	if err != nil {
		log.Fatal(err)
	}

	ext := platforms.DetermineExecutableFilenameSuffix()

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	os.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel."+osName+"-"+arch+ext), []byte(""), 0700)
	// Also create the standard wrapper to ensure we prefer the os/arch-specific wrapper.
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel"+ext), []byte(""), 0700)
	// Also create the plaform-specific wrapper to ensure we prefer the os/arch-specific wrapper.
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel."+arch+ext), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())
	expected := filepath.Join(tmpDir, "tools", "bazel."+osName+"-"+arch+ext)

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}

func TestMaybeDelegateToArchSpecificWrapper(t *testing.T) {
	var tmpDir, err = os.MkdirTemp("", "TestMaybeDelegateToArchSpecificWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	osName, err := platforms.DetermineOperatingSystem()
	if err != nil {
		log.Fatal(err)
	}
	arch, err := platforms.DetermineArchitecture(osName, platforms.DarwinArm64MinVersion)
	if err != nil {
		log.Fatal(err)
	}

	ext := platforms.DetermineExecutableFilenameSuffix()

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	os.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel."+arch+ext), []byte(""), 0700)
	// Also create the standard wrapper to ensure we prefer the arch-specific wrapper.
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel"+ext), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())
	expected := filepath.Join(tmpDir, "tools", "bazel."+arch+ext)

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}

func TestMaybeDelegateToStandardWrapper(t *testing.T) {
	var tmpDir, err = os.MkdirTemp("", "TestMaybeDelegateToStandardWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ext := platforms.DetermineExecutableFilenameSuffix()

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	os.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel"+ext), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())
	expected := filepath.Join(tmpDir, "tools", "bazel"+ext)

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

func TestMaybeDelegateToPowershellOverNoExtensionWrapper(t *testing.T) {
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
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel"), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())

	// On Windows a PowerShell script should be chosen over the wrapper without an extension
	var expected string
	if runtime.GOOS == "windows" {
		expected = filepath.Join(tmpDir, "tools", "bazel.ps1")
	} else {
		expected = filepath.Join(tmpDir, "tools", "bazel")
	}

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}

func TestMaybeDelegateToBatchOverNoExtensionWrapper(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "TestMaybeDelegateToPowershellOverBatchWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	os.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel.bat"), []byte(""), 0700)
	os.WriteFile(filepath.Join(tmpDir, "tools", "bazel"), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())

	// On Windows a batch script should be chosen over the wrapper without an extension
	var expected string
	if runtime.GOOS == "windows" {
		expected = filepath.Join(tmpDir, "tools", "bazel.bat")
	} else {
		expected = filepath.Join(tmpDir, "tools", "bazel")
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

func TestPrependDirToPathListWithEqualsInPath(t *testing.T) {
	cmd := &exec.Cmd{
		Env: []string{
			"PATH=/path/with=equals:/another/path",
			"OTHER_VAR=value",
		},
	}

	prependDirToPathList(cmd, "/new/dir")

	// Check that PATH was properly updated
	found := false
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "PATH=") {
			found = true
			// Should contain the new directory at the beginning
			if !strings.Contains(env, "/new/dir") {
				t.Errorf("Expected PATH to contain '/new/dir', got: %s", env)
			}
			// Should contain the original PATH value
			if !strings.Contains(env, "/path/with=equals:/another/path") {
				t.Errorf("Expected PATH to contain original value, got: %s", env)
			}
			break
		}
	}

	if !found {
		t.Error("PATH environment variable not found")
	}
}

func TestRunBazeliskWithStderrRedirection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows due to executable format requirements")
	}

	tmpDir, err := os.MkdirTemp("", "TestRunBazeliskWithStderrRedirection")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	bazelDir := filepath.Join(tmpDir, "bazel")
	err = os.MkdirAll(bazelDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create bazel dir: %v", err)
	}

	mockBazelPath := filepath.Join(bazelDir, "bazel")
	scriptContent := "#!/bin/sh\necho 'stdout message'\necho 'stderr message' >&2\nexit 0\n"

	err = os.WriteFile(mockBazelPath, []byte(scriptContent), 0755)
	if err != nil {
		t.Fatalf("Failed to create mock bazel: %v", err)
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	cfg := config.Static(map[string]string{
		"USE_BAZEL_VERSION": mockBazelPath,
		"BAZELISK_HOME":     tmpDir,
	})

	argsFunc := func(_ string) []string {
		return []string{"version"}
	}

	repos := CreateRepositories(nil, nil, nil, nil, false)

	exitCode, err := RunBazeliskWithArgsFuncAndConfigAndOutAndErr(argsFunc, repos, cfg, &stdoutBuf, &stderrBuf)
	if err != nil {
		t.Fatalf("RunBazeliskWithArgsFuncAndConfigAndOutAndErr failed: %v", err)
	}

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	stdoutContent := stdoutBuf.String()
	if !strings.Contains(stdoutContent, "stdout message") {
		t.Errorf("Expected stdout to contain 'stdout message', got: %q", stdoutContent)
	}

	stderrContent := stderrBuf.String()
	if !strings.Contains(stderrContent, "stderr message") {
		t.Errorf("Expected stderr to contain 'stderr message', got: %q", stderrContent)
	}

	if strings.Contains(stdoutContent, "stderr message") {
		t.Error("stderr content should not appear in stdout")
	}

	if strings.Contains(stderrContent, "stdout message") {
		t.Error("stdout content should not appear in stderr")
	}
}

func TestGetExpectedSHA256(t *testing.T) {
	testCases := []struct {
		name     string
		config   map[string]string
		osName   string
		arch     string
		nojdk    bool
		expected string
	}{
		{
			name: "platform-specific takes precedence",
			config: map[string]string{
				"BAZELISK_VERIFY_SHA256":              "generic-hash",
				"BAZELISK_VERIFY_SHA256_LINUX_X86_64": "platform-hash",
			},
			osName:   "linux",
			arch:     "x86_64",
			nojdk:    false,
			expected: "platform-hash",
		},
		{
			name: "falls back to generic when platform-specific not set",
			config: map[string]string{
				"BAZELISK_VERIFY_SHA256": "generic-hash",
			},
			osName:   "darwin",
			arch:     "arm64",
			nojdk:    false,
			expected: "generic-hash",
		},
		{
			name:     "empty when neither set",
			config:   map[string]string{},
			osName:   "windows",
			arch:     "x86_64",
			nojdk:    false,
			expected: "",
		},
		{
			name: "platform hash casing is preserved",
			config: map[string]string{
				"BAZELISK_VERIFY_SHA256_LINUX_ARM64": "AbCdEf123456",
			},
			osName:   "linux",
			arch:     "arm64",
			nojdk:    false,
			expected: "AbCdEf123456",
		},
		// nojdk test cases
		{
			name:   "generic hash casing is preserved",
			config: map[string]string{"BAZELISK_VERIFY_SHA256": "AbCdEf"},
			osName: "linux", arch: "x86_64", expected: "AbCdEf",
		},
		{
			name:   "NOJDK hash casing is preserved",
			config: map[string]string{"BAZELISK_VERIFY_SHA256_NOJDK_LINUX_X86_64": "AbCdEf"},
			osName: "linux", arch: "x86_64", nojdk: true, expected: "AbCdEf",
		},
		{
			name: "nojdk-specific takes precedence when nojdk enabled",
			config: map[string]string{
				"BAZELISK_VERIFY_SHA256":                    "generic-hash",
				"BAZELISK_VERIFY_SHA256_LINUX_X86_64":       "platform-hash",
				"BAZELISK_VERIFY_SHA256_NOJDK_LINUX_X86_64": "nojdk-hash",
			},
			osName:   "linux",
			arch:     "x86_64",
			nojdk:    true,
			expected: "nojdk-hash",
		},
		{
			name: "nojdk falls back to platform-specific when nojdk key not set",
			config: map[string]string{
				"BAZELISK_VERIFY_SHA256":              "generic-hash",
				"BAZELISK_VERIFY_SHA256_LINUX_X86_64": "platform-hash",
			},
			osName:   "linux",
			arch:     "x86_64",
			nojdk:    true,
			expected: "platform-hash",
		},
		{
			name: "nojdk key ignored when nojdk disabled",
			config: map[string]string{
				"BAZELISK_VERIFY_SHA256":                    "generic-hash",
				"BAZELISK_VERIFY_SHA256_LINUX_X86_64":       "platform-hash",
				"BAZELISK_VERIFY_SHA256_NOJDK_LINUX_X86_64": "nojdk-hash",
			},
			osName:   "linux",
			arch:     "x86_64",
			nojdk:    false,
			expected: "platform-hash",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Static(tc.config)
			result := getExpectedSHA256(cfg, tc.osName, tc.arch, tc.nojdk)
			if result != tc.expected {
				t.Errorf("getExpectedSHA256() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestGetExpectedSHA256LayeredConfig(t *testing.T) {
	cfg := config.Layered(
		config.Static(map[string]string{"BAZELISK_VERIFY_SHA256": "generic-from-env"}),
		config.Static(map[string]string{"BAZELISK_VERIFY_SHA256_LINUX_X86_64": "platform-from-file"}),
	)
	if got := getExpectedSHA256(cfg, "linux", "x86_64", false); got != "platform-from-file" {
		t.Errorf("selected hash %q, want platform-from-file", got)
	}
}

func TestGetExpectedSHA256DarwinFallback(t *testing.T) {
	cfg := config.Static(map[string]string{
		"BAZELISK_VERIFY_SHA256_DARWIN_ARM64":  "arm64-hash",
		"BAZELISK_VERIFY_SHA256_DARWIN_X86_64": "x86_64-hash",
	})
	arch := platforms.DarwinFallback("arm64", "4.0.1")
	if got := getExpectedSHA256(cfg, "darwin", arch, false); got != "x86_64-hash" {
		t.Errorf("selected hash %q, want x86_64-hash", got)
	}
}
