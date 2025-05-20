// Package ws offers functions to get information about Bazel workspaces.
package ws

import (
	"os"
	"path/filepath"
)

// FindWorkspaceRoot returns the root directory of the Bazel workspace in which the passed root exists, if any.
func FindWorkspaceRoot(root string) string {
	for _, boundary := range [...]string{"MODULE.bazel", "REPO.bazel", "WORKSPACE.bazel", "WORKSPACE"} {
		if isValidWorkspace(filepath.Join(root, boundary)) {
			return root
		}
	}

	parentDirectory := filepath.Dir(root)
	if parentDirectory == root {
		return ""
	}

	return FindWorkspaceRoot(parentDirectory)
}

// isValidWorkspace returns true if the supplied path is the workspace root, defined by the presence of
// a file named MODULE.bazel, REPO.bazel, WORKSPACE.bazel, or WORKSPACE
// see https://github.com/bazelbuild/bazel/blob/7.2.1/src/main/cpp/workspace_layout.cc#L34
func isValidWorkspace(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}
