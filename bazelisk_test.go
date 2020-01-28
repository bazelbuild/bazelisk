package main

import (
	"os"
	"testing"
)

func TestVersionStringForCommit(t *testing.T)  {
	versonString := "foo/commit/aabbccdd"
	info, err := parseBazelForkAndVersion(versonString)

	if err != nil {
		t.Errorf("failed to parse valid version string %s: %v", versonString, err)
	}

	if !info.IsSourceReference {
		t.Errorf("expected source build from version string %s", versonString)
	}

	if info.Fork != "foo" {
		t.Errorf("fork '%s' does not match expected fork 'foo'", info.Fork)
	}

	if info.VersionOrCommit != "aabbccdd" {
		t.Errorf("commit sha '%s' does not match expected commit sha 'faabbccdd'", info.VersionOrCommit)
	}
}

func TestVersionStringForCommitWithBadPattern(t *testing.T)  {
	versonString := "foo/cmit/aabbccdd"
	_, err := parseBazelForkAndVersion(versonString)

	if err == nil {
		t.Errorf("expected error when parsing invalid version string %s", versonString)
	}
}

func TestDetermineSourceURL(t *testing.T) {
	url := determineSourceURL("foo")

	if url != "ssh://git@github.com/foo/bazel.git" {
		t.Errorf("url without BAZELISK_BASE_URL '%s' does not match expected 'ssh://git@github.com/foo/bazel.git'", url)
	}

	if err := os.Setenv(bazelURLEnv, "ssh://fizz@fizzhub.com"); err != nil {
		t.Errorf("failed to set %s env variable for test: %v", bazelURLEnv, err)
	}

	url = determineSourceURL("baz")

	if url != "ssh://fizz@fizzhub.com/baz/bazel.git" {
		t.Errorf("url with BAZELISK_BASE_URL set '%s' does not match expected 'ssh://fizz@fizzhub.com/baz/bazel.git'", url)
	}
}