package ws

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindFileInParentDirectory(t *testing.T) {
	root := t.TempDir()
	workingDirectory := filepath.Join(root, "nested", "directory")
	if err := os.MkdirAll(workingDirectory, 0700); err != nil {
		t.Fatalf("Failed to create working directory: %v", err)
	}

	want := filepath.Join(root, ".bazelversion")
	if err := os.WriteFile(want, nil, 0600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	if got := FindFile(workingDirectory, ".bazelversion"); got != want {
		t.Fatalf("FindFile returned %q, want %q", got, want)
	}
}

func TestFindFilePrefersNearestFile(t *testing.T) {
	root := t.TempDir()
	workingDirectory := filepath.Join(root, "nested")
	if err := os.Mkdir(workingDirectory, 0700); err != nil {
		t.Fatalf("Failed to create working directory: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, ".bazelversion"), nil, 0600); err != nil {
		t.Fatalf("Failed to write parent file: %v", err)
	}

	rootFile := filepath.Join(root, ".bazelversion")
	if got := FindFile(workingDirectory, ".bazelversion"); got != rootFile {
		t.Fatalf("FindFile returned %q, want %q", got, rootFile)
	}

	nestedFile := filepath.Join(workingDirectory, ".bazelversion")
	if err := os.WriteFile(nestedFile, nil, 0600); err != nil {
		t.Fatalf("Failed to write nearest file: %v", err)
	}

	if got := FindFile(workingDirectory, ".bazelversion"); got != nestedFile {
		t.Fatalf("FindFile returned %q, nestedFile %q", got, nestedFile)
	}
}

func TestFindFileReturnsEmptyWhenFileDoesNotExist(t *testing.T) {
	if got := FindFile(t.TempDir(), ".bazelversion"); got != "" {
		t.Fatalf("FindFile returned %q, want an empty path", got)
	}
}

func TestFindWorkspaceRootRecognizesWorkspaceBoundaries(t *testing.T) {
	for _, boundary := range []string{"MODULE.bazel", "REPO.bazel", "WORKSPACE.bazel", "WORKSPACE"} {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			workingDirectory := filepath.Join(root, "nested", "directory")
			if err := os.MkdirAll(workingDirectory, 0700); err != nil {
				t.Fatalf("Failed to create working directory: %v", err)
			}
			if err := os.WriteFile(filepath.Join(root, boundary), nil, 0600); err != nil {
				t.Fatalf("Failed to write workspace boundary: %v", err)
			}

			if got := FindWorkspaceRoot(workingDirectory); got != root {
				t.Fatalf("FindWorkspaceRoot returned %q, want %q", got, root)
			}
		})
	}
}

func TestFindWorkspaceRootPrefersNearestWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "WORKSPACE"), nil, 0600); err != nil {
		t.Fatalf("Failed to write outer workspace boundary: %v", err)
	}

	want := filepath.Join(root, "nested")
	workingDirectory := filepath.Join(want, "directory")
	if err := os.MkdirAll(workingDirectory, 0700); err != nil {
		t.Fatalf("Failed to create working directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(want, "MODULE.bazel"), nil, 0600); err != nil {
		t.Fatalf("Failed to write inner workspace boundary: %v", err)
	}

	if got := FindWorkspaceRoot(workingDirectory); got != want {
		t.Fatalf("FindWorkspaceRoot returned %q, want %q", got, want)
	}
}

func TestFindWorkspaceRootReturnsEmptyOutsideWorkspace(t *testing.T) {
	if got := FindWorkspaceRoot(t.TempDir()); got != "" {
		t.Fatalf("FindWorkspaceRoot returned %q, want an empty path", got)
	}
}

func TestFindWorkspaceRootIgnoresDirectoryBoundary(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "MODULE.bazel"), 0700); err != nil {
		t.Fatalf("Failed to create boundary directory: %v", err)
	}

	if got := FindWorkspaceRoot(root); got != "" {
		t.Fatalf("FindWorkspaceRoot returned %q, want an empty path", got)
	}
}
