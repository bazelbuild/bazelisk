package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocateWorkspaceConfigFileInParentDirectory(t *testing.T) {
	root := t.TempDir()
	workingDirectory := filepath.Join(root, "nested", "directory")
	if err := os.MkdirAll(workingDirectory, 0700); err != nil {
		t.Fatalf("Failed to create working directory: %v", err)
	}

	want := filepath.Join(root, rcFileName)
	if err := os.WriteFile(want, nil, 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	t.Chdir(workingDirectory)

	got, err := LocateWorkspaceConfigFile()
	if err != nil {
		t.Fatalf("LocateWorkspaceConfigFile failed unexpectedly: %v", err)
	}
	if got != want {
		t.Fatalf("LocateWorkspaceConfigFile returned %q, want %q", got, want)
	}
}
