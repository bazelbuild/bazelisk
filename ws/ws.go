package ws

import (
	"os"
	"path/filepath"
)

// FindWorkspaceRoot returns the root directory of the Bazel workspace in which the passed root exists, if any.
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
