// Package core.configs contains the core.configs Bazelisk logic, as well as abstractions for Bazel repositories.
package configs

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bazelbuild/bazelisk/core/path"
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

	// Parse .bazeliskrc in the workspace root, once, if it can be found.
	fileConfigOnce.Do(func() {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return
		}
		workspaceRoot := path.FindWorkspaceRoot(workingDirectory)
		if workspaceRoot == "" {
			return
		}
		rcFilePath := filepath.Join(workspaceRoot, ".bazeliskrc")
		contents, err := ioutil.ReadFile(rcFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			log.Fatal(err)
		}
		fileConfig = make(map[string]string)
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
			fileConfig[key] = strings.TrimSpace(parts[1])
		}
	})

	return fileConfig[name]
}
