// Package ws offers functions for locating Bazel workspaces and related files.
package ws

import (
	"os"
	"path/filepath"
)

// FindFile returns the nearest file at relativePath in startDirectory or one of
// its parent directories. It returns an empty string if no such file exists.
func FindFile(startDirectory string, relativePath string) string {
	directory := startDirectory
	for {
		path := filepath.Join(directory, relativePath)
		if isFile(path) {
			return path
		}

		parentDirectory := filepath.Dir(directory)
		if parentDirectory == directory {
			return ""
		}
		directory = parentDirectory
	}
}

// FindWorkspaceRoot returns the root directory of the Bazel workspace in which
// root exists, if any. Defined by the presence of
// a file named MODULE.bazel, REPO.bazel, WORKSPACE.bazel, or WORKSPACE
// see https://github.com/bazelbuild/bazel/blob/7.2.1/src/main/cpp/workspace_layout.cc#L34
func FindWorkspaceRoot(root string) string {
	for _, boundary := range [...]string{"MODULE.bazel", "REPO.bazel", "WORKSPACE.bazel", "WORKSPACE"} {
		if isFile(filepath.Join(root, boundary)) {
			return root
		}
	}

	parentDirectory := filepath.Dir(root)
	if parentDirectory == root {
		return ""
	}

	return FindWorkspaceRoot(parentDirectory)
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}
