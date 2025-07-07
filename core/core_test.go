package core

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bazelbuild/bazelisk/platforms" // Required for determining bazel file names
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaybeDelegateToNoWrapper(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "TestMaybeDelegateToNoWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir | 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, true)
	expected := "bazel_real"

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}

// Note: Additional imports will be needed:
// import (
// 	"fmt"
// 	"io"
// 	"os/exec"
// 	"strings"
// 	"syscall"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// 	"github.com/bazelbuild/bazelisk/platforms"
// 	"github.com/bazelbuild/bazelisk/versions" // May not be directly needed in test but related
// )
// For brevity, I'm not adding them in this diff but they are necessary for the code to compile.


func TestDownloadBazelIfNecessary_CompletionScriptGeneration(t *testing.T) {
	// Helper function to mock runBazelInternal
	originalRunBazelInternal := runBazelInternal // Store original
	defer func() { runBazelInternal = originalRunBazelInternal }() // Restore original

	// Define a default mock for runBazelInternal that can be overridden by tests
	var currentMockRunBazelInternal func(string, []string, io.Writer, bool) (int, error)
	runBazelInternal = func(bazelPath string, args []string, out io.Writer, useSystemEnv bool) (int, error) {
		if currentMockRunBazelInternal != nil {
			return currentMockRunBazelInternal(bazelPath, args, out, useSystemEnv)
		}
		// Default behavior if no specific mock is set for the test case
		if len(args) == 3 && args[0] == "help" && args[1] == "complete" && args[2] == "bash" {
			// Simulate error for unexpected calls to prevent tests from passing accidentally
			return 1, fmt.Errorf("mockRunBazelInternal called unexpectedly for %s with args %v", bazelPath, args)
		}
		return 0, nil // Default success for other calls if any
	}

	tests := []struct {
		name                     string
		bazelVersionToDownload   string // Version string passed to downloadBazelIfNecessary
		mockedBazelVersionInPath string // Version string embedded in the path for mockRunBazelInternal to key off
		downloaderShouldErr      bool
		expectedScriptContent    string
		expectScriptExists       bool
		setupMockInternalRunner  func(t *testing.T, tcName string, mockedVersionInPath string)
		expectedLogContains      []string
	}{
		{
			name:                     "Bazel 8.4.0 - script generated",
			bazelVersionToDownload:   "8.4.0",
			mockedBazelVersionInPath: "8.4.0",
			expectedScriptContent:    "fake completion script for 8.4.0",
			expectScriptExists:       true,
			setupMockInternalRunner: func(t *testing.T, tcName string, mockedVersionInPath string) {
				currentMockRunBazelInternal = func(bazelPath string, args []string, out io.Writer, useSystemEnv bool) (int, error) {
					if len(args) == 3 && args[0] == "help" && args[1] == "complete" && args[2] == "bash" {
						if strings.Contains(bazelPath, mockedVersionInPath) {
							_, err := out.Write([]byte("fake completion script for " + mockedVersionInPath))
							assert.NoError(t, err, tcName+": mock writing to out failed")
							return 0, nil
						}
					}
					return 1, fmt.Errorf("[%s] unexpected call to mockRunBazelInternal: %s %v", tcName, bazelPath, args)
				}
			},
		},
		{
			name:                     "Bazel 9.0.0 - script generated",
			bazelVersionToDownload:   "9.0.0",
			mockedBazelVersionInPath: "9.0.0",
			expectedScriptContent:    "fake completion script for 9.0.0",
			expectScriptExists:       true,
			setupMockInternalRunner: func(t *testing.T, tcName string, mockedVersionInPath string) {
				currentMockRunBazelInternal = func(bazelPath string, args []string, out io.Writer, useSystemEnv bool) (int, error) {
					if len(args) == 3 && args[0] == "help" && args[1] == "complete" && args[2] == "bash" {
						if strings.Contains(bazelPath, mockedVersionInPath) {
							_, err := out.Write([]byte("fake completion script for " + mockedVersionInPath))
							assert.NoError(t, err, tcName+": mock writing to out failed")
							return 0, nil
						}
					}
					return 1, fmt.Errorf("[%s] unexpected call to mockRunBazelInternal: %s %v", tcName, bazelPath, args)
				}
			},
		},
		{
			name:                     "Bazel 7.0.0 - no script (version too old)",
			bazelVersionToDownload:   "7.0.0",
			mockedBazelVersionInPath: "7.0.0",
			expectScriptExists:       false,
			setupMockInternalRunner: func(t *testing.T, tcName string, mockedVersionInPath string) {
				currentMockRunBazelInternal = func(bazelPath string, args []string, out io.Writer, useSystemEnv bool) (int, error) {
					// This should not be called for script generation
					return 1, fmt.Errorf("[%s] mockRunBazelInternal for help complete bash should not be called for old version %s", tcName, mockedVersionInPath)
				}
			},
			expectedLogContains: []string{"Skipping completion script generation"},
		},
		{
			name:                     "Bazel 8.4.0 - generation fails (runBazelInternal error)",
			bazelVersionToDownload:   "8.4.1", // Use a unique version for this test's mock
			mockedBazelVersionInPath: "8.4.1",
			expectScriptExists:       false,
			setupMockInternalRunner: func(t *testing.T, tcName string, mockedVersionInPath string) {
				currentMockRunBazelInternal = func(bazelPath string, args []string, out io.Writer, useSystemEnv bool) (int, error) {
					if len(args) == 3 && args[0] == "help" && args[1] == "complete" && args[2] == "bash" {
						if strings.Contains(bazelPath, mockedVersionInPath) {
							return 1, fmt.Errorf("simulated 'help complete bash' error")
						}
					}
					return 1, fmt.Errorf("[%s] unexpected call to mockRunBazelInternal: %s %v", tcName, bazelPath, args)
				}
			},
			expectedLogContains: []string{"Warning: could not run", "help complete bash", "to generate completion script"},
		},
		{
			name:                     "Bazel 8.4.0 - empty output from 'help complete bash'",
			bazelVersionToDownload:   "8.4.2", // Unique version for this test's mock
			mockedBazelVersionInPath: "8.4.2",
			expectScriptExists:       false,
			setupMockInternalRunner: func(t *testing.T, tcName string, mockedVersionInPath string) {
				currentMockRunBazelInternal = func(bazelPath string, args []string, out io.Writer, useSystemEnv bool) (int, error) {
					if len(args) == 3 && args[0] == "help" && args[1] == "complete" && args[2] == "bash" {
						if strings.Contains(bazelPath, mockedVersionInPath) {
							// Simulate success but no output
							return 0, nil
						}
					}
					return 1, fmt.Errorf("[%s] unexpected call to mockRunBazelInternal: %s %v", tcName, bazelPath, args)
				}
			},
			expectedLogContains: []string{"Warning:", "produced empty output", "Not saving completion script"},
		},
		{
			name:                     "Commit hash (non-semantic) - no script",
			bazelVersionToDownload:   "abcdef1234567890abcdef1234567890abcdef12",
			mockedBazelVersionInPath: "abcdef1234567890abcdef1234567890abcdef12", // Should not be used in path this way
			expectScriptExists:       false,
			setupMockInternalRunner: func(t *testing.T, tcName string, mockedVersionInPath string) {
				currentMockRunBazelInternal = func(bazelPath string, args []string, out io.Writer, useSystemEnv bool) (int, error) {
					return 1, fmt.Errorf("[%s] mockRunBazelInternal for help complete bash should not be called for non-semantic version", tcName)
				}
			},
			expectedLogContains: []string{"Skipping completion script generation for non-semantic version"},
		},
		{
			name:                   "Downloader error - no script, no attempt to generate",
			bazelVersionToDownload: "8.5.0",
			downloaderShouldErr:    true,
			expectScriptExists:     false,
			// No runner setup needed as download will fail first
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setupMockInternalRunner != nil {
				tc.setupMockInternalRunner(t, tc.name, tc.mockedBazelVersionInPath)
			} else {
				// Default mock if none specified, to catch unexpected calls
				currentMockRunBazelInternal = func(s string, s2 []string, writer io.Writer, b bool) (int, error) {
					return 1, fmt.Errorf("[%s] default mock for runBazelInternal called unexpectedly", tc.name)
				}
			}

			tempHome := t.TempDir()
			// baseDirectory is like $BAZELISK_HOME/downloads/<fork>
			// downloadBazelIfNecessary will append <platform_specific_path>/bin/bazel to it.
			// The <platform_specific_path> is what contains the version string.
			baseDirectoryForTest := filepath.Join(tempHome, "downloads", "bazelbuild_test_fork")
			err := os.MkdirAll(baseDirectoryForTest, 0755)
			require.NoError(t, err, tc.name+": Failed to create baseDirectoryForTest")

			// Capture log output
			var logOutput bytes.Buffer
			log.SetOutput(&logOutput)
			defer log.SetOutput(os.Stderr) // Reset log output

			downloader := func(destDir, destFile string) (string, error) {
				if tc.downloaderShouldErr {
					return "", fmt.Errorf("simulated downloader error")
				}
				finalDestPath := filepath.Join(destDir, destFile)
				err := os.MkdirAll(filepath.Dir(finalDestPath), 0755)
				require.NoError(t, err, tc.name+": Failed to create dir for dummy bazel: "+filepath.Dir(finalDestPath))
				// Create a dummy file to represent the downloaded bazel
				f, err := os.Create(finalDestPath)
				require.NoError(t, err, tc.name+": Failed to create dummy bazel: "+finalDestPath)
				f.Close()
				return finalDestPath, nil
			}

			// The repos argument is not strictly used by downloadBazelIfNecessary when a downloader func is provided.
			repos := &Repositories{}

			// The version string passed here (tc.bazelVersionToDownload) is used for:
			// 1. Version check (>= 8.4.0)
			// 2. Passed to platforms.DetermineBazelFilename to create the path segment.
			// The mock for runBazelInternal uses tc.mockedBazelVersionInPath to check if it's being called
			// for the right "downloaded" bazel, by checking if tc.mockedBazelVersionInPath is in the bazelPath.
			actualBazelPath, err := downloadBazelIfNecessary(tc.bazelVersionToDownload, baseDirectoryForTest, repos, downloader)

			if tc.downloaderShouldErr {
				assert.Error(t, err, tc.name+": Expected downloader error")
				return // Test ends here if download fails
			}
			assert.NoError(t, err, tc.name+": downloadBazelIfNecessary failed unexpectedly")
			require.NotEmpty(t, actualBazelPath, tc.name+": actualBazelPath should not be empty")


			completionScriptPath := getCompletionScriptPath(actualBazelPath)
			_, statErr := os.Stat(completionScriptPath)

			if tc.expectScriptExists {
				assert.NoError(t, statErr, tc.name+": Completion script should exist at %s", completionScriptPath)
				content, readErr := ioutil.ReadFile(completionScriptPath)
				assert.NoError(t, readErr, tc.name+": Failed to read completion script")
				assert.Equal(t, tc.expectedScriptContent, string(content), tc.name+": Script content mismatch")
			} else {
				assert.True(t, os.IsNotExist(statErr), tc.name+": Completion script should NOT exist at %s, but got err: %v", completionScriptPath, statErr)
			}

			logStr := logOutput.String()
			for _, expectedLog := range tc.expectedLogContains {
				assert.Contains(t, logStr, expectedLog, tc.name+": Log output mismatch")
			}
		})
	}
}

// TestBazeliskCompleteCommand needs to be added here too.
// For now, just adding the one test function.

func TestBazeliskCompleteCommand(t *testing.T) {
	originalGetEnvOrConfig := GetEnvOrConfig
	defer func() { GetEnvOrConfig = originalGetEnvOrConfig }()

	originalUserCacheDir := os.UserCacheDir
	defer func() { os.UserCacheDir = originalUserCacheDir }()


	// Mock Repositories and its methods
	mockRepos := &Repositories{
		ResolveVersionFunc: func(bazeliskHome, fork, version string) (string, DownloadFunc, error) {
			// For 'complete' command, we often resolve to a specific version string like "8.4.0"
			// The actual download func might not be called if we place the file manually.
			if version == "8.4.0" || version == "test-version-exists" {
				return version, func(destDir, destFile string) (string, error) {
					// Mock downloader, creates a dummy bazel executable
					bazelPath := filepath.Join(destDir, destFile)
					err := os.MkdirAll(filepath.Dir(bazelPath), 0755)
					require.NoError(t, err)
					f, err := os.Create(bazelPath)
					require.NoError(t, err)
					f.Close()
					// Also pre-create the completion script if needed for the test case
					if strings.Contains(version, "exists") {
						scriptPath := getCompletionScriptPath(bazelPath)
						err = ioutil.WriteFile(scriptPath, []byte("fake completion content for "+version), 0644)
						require.NoError(t, err)
					}
					return bazelPath, nil
				}, nil
			}
			if version == "test-version-no-script" {
				return version, func(destDir, destFile string) (string, error) {
					bazelPath := filepath.Join(destDir, destFile)
					err := os.MkdirAll(filepath.Dir(bazelPath), 0755)
					require.NoError(t, err)
					f, err := os.Create(bazelPath)
					require.NoError(t, err)
					f.Close()
					// DO NOT create completion script
					return bazelPath, nil
				}, nil
			}
			if version == "non-existent-version" {
				return "", nil, fmt.Errorf("simulated resolve error for non-existent-version")
			}
			return version, nil, fmt.Errorf("unmocked ResolveVersion call: %s, %s", fork, version)
		},
	}


	tests := []struct {
		name               string
		args               []string
		setupEnvVars       map[string]string // For GetEnvOrConfig
		setupBazelVersion  string            // Content of .bazelversion file
		bazelVersionForURL string            // Version to use if URL needs to be constructed
		expectedExitCode   int
		expectedStdout     string
		expectedStderr     string // Check if stderr contains this string
		tempDirSetup       func(t *testing.T, tempDir string, testCaseBazelVersion string) // For creating .bazelversion or other files
	}{
		{
			name: "Completion script exists",
			args: []string{"complete"},
			setupEnvVars: map[string]string{
				"USE_BAZEL_VERSION": "test-version-exists", // This version will have its script pre-created by mock downloader
			},
			expectedExitCode: 0,
			expectedStdout:   "fake completion content for test-version-exists",
		},
		{
			name: "Completion script does not exist",
			args: []string{"complete"},
			setupEnvVars: map[string]string{
				"USE_BAZEL_VERSION": "test-version-no-script", // This version will NOT have its script
			},
			expectedExitCode: 1,
			expectedStderr:   "Error: Bash completion script not found for Bazel version test-version-no-script",
		},
		{
			name: "Bazel version from .bazelversion file - script exists",
			args: []string{"complete"},
			tempDirSetup: func(t *testing.T, tempDir string, testCaseBazelVersion string) {
				// The mock downloader for "test-version-exists" will create the script.
				err := ioutil.WriteFile(filepath.Join(tempDir, ".bazelversion"), []byte("test-version-exists"), 0644)
				require.NoError(t, err)
				// Create WORKSPACE to make it a workspace root
				err = ioutil.WriteFile(filepath.Join(tempDir, "WORKSPACE"), []byte(""), 0644)
				require.NoError(t, err)
			},
			expectedExitCode: 0,
			expectedStdout:   "fake completion content for test-version-exists",
		},
		{
			name: "Bazel version resolution fails",
			args: []string{"complete"},
			setupEnvVars: map[string]string{
				"USE_BAZEL_VERSION": "non-existent-version",
			},
			expectedExitCode: -1, // RunBazeliskWithArgsFunc returns -1 and an error for this
			expectedStderr:   "could not resolve Bazel path for complete command", // Error from our 'complete' logic
		},
		{
			name: "No bazel version specified - uses fallback, script missing",
			args: []string{"complete"},
			// default fallback is "latest", our mock doesn't handle "latest" specifically for script creation
			// so it will be treated like "test-version-no-script" effectively by not finding a script.
			// We need to adjust the mock to handle "latest" or make the test more specific.
			// For now, let's assume "latest" resolves to something our mock can provide a bazel binary for, but no script.
			setupEnvVars:       map[string]string{
				// No USE_BAZEL_VERSION, will use fallback logic.
				// The mock for ResolveVersion needs to handle "latest" or whatever the fallback is.
				// Let's make the fallback point to a version we control in the mock.
				"USE_BAZEL_FALLBACK_VERSION": "silent:test-version-no-script",
			},
			expectedExitCode: 1,
			expectedStderr:   "Error: Bash completion script not found for Bazel version test-version-no-script",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempHome := t.TempDir() // This will be BAZELISK_HOME
			workspaceDir := t.TempDir() // This will be the CWD

			// Set up environment variables for GetEnvOrConfig
			GetEnvOrConfig = func(name string) string {
				if val, ok := tc.setupEnvVars[name]; ok {
					return val
				}
				if name == "BAZELISK_HOME" {
					return tempHome
				}
				// Provide default for UserCacheDir if BAZELISK_HOME is not set (though it is in these tests)
				if name == "" && os.Getenv("BAZELISK_HOME") == "" { // Heuristic for UserCacheDir context
					return filepath.Join(tempHome, "user_cache_dir_for_test")
				}
				return originalGetEnvOrConfig(name) // Fallback to original for other vars
			}

			os.UserCacheDir = func() (string, error) {
				return filepath.Join(tempHome, "user_cache_dir_for_test"), nil
			}


			if tc.tempDirSetup != nil {
				tc.tempDirSetup(t, workspaceDir, tc.setupBazelVersion)
			}

			// Change CWD for the test
			originalCwd, err := os.Getwd()
			require.NoError(t, err)
			err = os.Chdir(workspaceDir)
			require.NoError(t, err)
			defer os.Chdir(originalCwd)


			// Capture stdout and stderr
			oldStdout := os.Stdout
			oldStderr := os.Stderr
			rOut, wOut, _ := os.Pipe()
			rErr, wErr, _ := os.Pipe()
			os.Stdout = wOut
			os.Stderr = wErr

			argsFunc := func(_ string) []string { return tc.args }
			exitCode, err := RunBazeliskWithArgsFunc(argsFunc, mockRepos)

			wOut.Close()
			wErr.Close()
			stdoutBytes, _ := ioutil.ReadAll(rOut)
			stderrBytes, _ := ioutil.ReadAll(rErr)
			os.Stdout = oldStdout
			os.Stderr = oldStderr

			assert.Equal(t, tc.expectedExitCode, exitCode, "Exit code mismatch")

			if tc.expectedStdout != "" {
				assert.Equal(t, tc.expectedStdout, string(stdoutBytes), "Stdout content mismatch")
			} else {
				assert.Empty(t, string(stdoutBytes), "Stdout should be empty")
			}

			if tc.expectedStderr != "" {
				assert.Contains(t, string(stderrBytes), tc.expectedStderr, "Stderr content mismatch")
			} else {
				// If we expect success (exit 0), stderr should ideally be empty or only contain specific warnings
				// For exit code 1 due to missing script, specific stderr is checked.
				// For other errors (exit -1), err will be non-nil.
				if tc.expectedExitCode == 0 {
					assert.Empty(t, string(stderrBytes), "Stderr should be empty on success")
				}
			}

			if exitCode == -1 && tc.expectedStderr != "" { // Indicates RunBazeliskWithArgsFunc returned an error
				require.Error(t, err, "Expected an error from RunBazeliskWithArgsFunc")
				assert.Contains(t, err.Error(), tc.expectedStderr, "Error message from RunBazeliskWithArgsFunc mismatch")
			} else if exitCode != -1 { // Indicates RunBazeliskWithArgsFunc itself didn't error, but might have set exit code
				require.NoError(t, err, "Expected no error from RunBazeliskWithArgsFunc itself if exit code is not -1")
			}
		})
	}
}

func TestMaybeDelegateToNoNonExecutableWrapper(t *testing.T) {
	// It's not guaranteed that `tools/bazel` is executable on the
	// Windows host running this test. Thus the test is skipped on
	// this platform to guarantee consistent results.
	if runtime.GOOS == "windows" {
		return
	}
	
	tmpDir, err := ioutil.TempDir("", "TestMaybeDelegateToNoNonExecutableWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir | 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir | 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel"), []byte(""), 0600)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, true)
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

	var tmpDir, err = ioutil.TempDir("", "TestMaybeDelegateToStandardWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir | 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir | 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel"), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, true)
	expected := filepath.Join(tmpDir, "tools", "bazel")

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}

func TestMaybeDelegateToPowershellWrapper(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "TestMaybeDelegateToPowershellWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir | 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir | 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel.ps1"), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, true)
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
	tmpDir, err := ioutil.TempDir("", "TestMaybeDelegateToBatchWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir | 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir | 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel.bat"), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, true)
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
	tmpDir, err := ioutil.TempDir("", "TestMaybeDelegateToPowershellOverBatchWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir | 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir | 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel.ps1"), []byte(""), 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel.bat"), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, true)
	expected := filepath.Join(tmpDir, "tools", "bazel.ps1")

	// Only windows platforms use powershell or batch wrappers
	if runtime.GOOS != "windows" {
		expected = "bazel_real"
	}

	if entrypoint != expected {
		t.Fatalf("Expected to delegate bazel to %q, but got %q", expected, entrypoint)
	}
}
