package core

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bazelbuild/bazelisk/config"
)

func TestMaybeDelegateToNoWrapper(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "TestMaybeDelegateToNoWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

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

	tmpDir, err := ioutil.TempDir("", "TestMaybeDelegateToNoNonExecutableWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel"), []byte(""), 0600)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())
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

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel"), []byte(""), 0700)

	entrypoint := maybeDelegateToWrapperFromDir("bazel_real", tmpDir, config.Null())
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

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel.ps1"), []byte(""), 0700)

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
	tmpDir, err := ioutil.TempDir("", "TestMaybeDelegateToBatchWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel.bat"), []byte(""), 0700)

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
	tmpDir, err := ioutil.TempDir("", "TestMaybeDelegateToPowershellOverBatchWrapper")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, os.ModeDir|0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "WORKSPACE"), []byte(""), 0600)
	ioutil.WriteFile(filepath.Join(tmpDir, "BUILD"), []byte(""), 0600)

	os.MkdirAll(filepath.Join(tmpDir, "tools"), os.ModeDir|0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel.ps1"), []byte(""), 0700)
	ioutil.WriteFile(filepath.Join(tmpDir, "tools", "bazel.bat"), []byte(""), 0700)

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
