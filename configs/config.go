// Package configs provides a mechanism to read configuration values from the environment and from .bazeliskrc files.
package configs

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	rcFileName = ".bazeliskrc"
)

var (
	fileConfig     map[string]string
	fileConfigOnce sync.Once
)

// GetEnvOrConfig reads a configuration value from the environment, but fall back to reading it from .bazeliskrc in the workspace root.
func GetEnvOrConfig(name string) string {
	if val := os.Getenv(name); val != "" {
		return val
	}

	fileConfigOnce.Do(loadFileConfig)

	return fileConfig[name]
}

// loadFileConfig locates available .bazeliskrc configuration files, parses them with a precedence order preference,
// and updates a global configuration map with their contents. This routine should be executed exactly once.
func loadFileConfig() {
	var rcFilePaths []string

	if userRC, err := locateUserConfigFile(); err == nil {
		rcFilePaths = append(rcFilePaths, userRC)
	}
	if workspaceRC, err := locateWorkspaceConfigFile(); err == nil {
		rcFilePaths = append(rcFilePaths, workspaceRC)
	}

	fileConfig = make(map[string]string)
	for _, rcPath := range rcFilePaths {
		config, err := parseFileConfig(rcPath)
		if err != nil {
			log.Fatal(err)
		}

		for key, value := range config {
			fileConfig[key] = value
		}
	}
}

// locateWorkspaceConfigFile locates a .bazeliskrc file in the current workspace root.
func locateWorkspaceConfigFile() (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	workspaceRoot := FindWorkspaceRoot(workingDirectory)
	if workspaceRoot == "" {
		return "", err
	}
	return filepath.Join(workspaceRoot, rcFileName), nil
}

// locateUserConfigFile locates a .bazeliskrc file in the user's home directory.
func locateUserConfigFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, rcFileName), nil
}

// parseFileConfig parses a .bazeliskrc file as a map of key-value configuration values.
func parseFileConfig(rcFilePath string) (map[string]string, error) {
	config := make(map[string]string)

	contents, err := ioutil.ReadFile(rcFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Non-critical error.
			return config, nil
		}
		return nil, err
	}

	for _, line := range strings.Split(string(contents), "\n") {
		if strings.HasPrefix(line, "#") {
			// comments
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		config[key] = strings.TrimSpace(parts[1])
	}

	return config, nil
}

// isValidWorkspace returns true iff the supplied path is the workspace root, defined by the presence of
// a file named WORKSPACE or WORKSPACE.bazel
// see https://github.com/bazelbuild/bazel/blob/8346ea4cfdd9fbd170d51a528fee26f912dad2d5/src/main/cpp/workspace_layout.cc#L37
func isValidWorkspace(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

// FindWorkspaceRoot returns root directory where the WORKSPACE file is located at.
func FindWorkspaceRoot(root string) string {
	if isValidWorkspace(filepath.Join(root, "WORKSPACE")) {
		return root
	}

	if isValidWorkspace(filepath.Join(root, "WORKSPACE.bazel")) {
		return root
	}

	parentDirectory := filepath.Dir(root)
	if parentDirectory == root {
		return ""
	}

	return FindWorkspaceRoot(parentDirectory)
}
