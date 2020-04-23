package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
)

var (
	transport = &fakeTransport{responses: make(map[string]*http.Response)}
	tmpDir    = ""
)

func init() {
	DefaultTransport = transport
}

func TestMain(m *testing.M) {
	var err error
	tmpDir, err = ioutil.TempDir("", "version_test")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	code := m.Run()
	os.Exit(code)
}

func TestResolveLatestVersion_UseGCSIfBazelGitHubIsDown_NoRC(t *testing.T) {
	transport.AddResponse("https://api.github.com/repos/bazelbuild/bazel/releases", 500, "")

	listBody := buildGCSResponseOrFail(t, []string{"4.0.0/", "12.0.0/", "10.0.0/"}, []interface{}{})
	transport.AddResponse("https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/", 200, listBody)

	rcBody := buildGCSResponseOrFail(t, []string{}, []interface{}{"this is a release"})
	transport.AddResponse("https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/&prefix=12.0.0/release/", 200, rcBody)

	version, err := resolveLatestVersion(tmpDir, bazelUpstream, 0)

	if err != nil {
		t.Fatalf("Version resolution failed unexpectedly: %v", err)
	}
	if version != "12.0.0" {
		t.Fatalf("Expected version 12.0.0, but got %s", version)
	}
}

func TestResolveLatestVersion_UseGCSIfBazelGitHubIsDown_WithRC(t *testing.T) {
	transport.AddResponse("https://api.github.com/repos/bazelbuild/bazel/releases", 500, "")

	listBody := buildGCSResponseOrFail(t, []string{"4.0.0/", "11.0.0/", "11.11.0/", "10.0.0/"}, []interface{}{})
	transport.AddResponse("https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/", 200, listBody)

	// 11.11.0 is the current RC, but the latest release is still 11.0.0
	rcBody := buildGCSResponseOrFail(t, []string{}, []interface{}{})
	transport.AddResponse("https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/&prefix=11.11.0/release/", 200, rcBody)

	version, err := resolveLatestVersion(tmpDir, bazelUpstream, 0)
	if err != nil {
		t.Fatalf("Version resolution failed unexpectedly: %v", err)
	}
	if version != "11.0.0" {
		t.Fatalf("Expected version 11.0.0, but got %s", version)
	}
}

func TestResolveLatestRcVersion(t *testing.T) {
	listBody := buildGCSResponseOrFail(t, []string{"4.0.0/", "11.0.0/", "11.11.0/", "10.0.0/"}, []interface{}{})
	transport.AddResponse("https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/", 200, listBody)

	rcListBody := buildGCSResponseOrFail(t, []string{"11.11.0/rc2/", "11.11.0/rc1/"}, []interface{}{})
	transport.AddResponse("https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/&prefix=11.11.0/", 200, rcListBody)

	// 11.11.0 is the current RC, but the latest release is still 11.0.0
	rcBody := buildGCSResponseOrFail(t, []string{}, []interface{}{})
	transport.AddResponse("https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/&prefix=11.11.0/release/", 200, rcBody)

	version, err := resolveLatestRcVersion()

	if err != nil {
		t.Fatalf("Version resolution failed unexpectedly: %v", err)
	}
	expectedRC := "11.11.0rc2"
	if version != expectedRC {
		t.Fatalf("Expected version %s, but got %s", expectedRC, version)
	}
}

func TestResolveLatestVersion_EverythingIsDown(t *testing.T) {
	transport.AddResponse("https://api.github.com/repos/bazelbuild/bazel/releases", 500, "")
	transport.AddResponse("https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/", 500, "")

	_, err := resolveLatestVersion(tmpDir, bazelUpstream, 0)

	if err == nil {
		t.Fatal("Expected resolveLatestVersion() to fail.")
	}
	expectedPrefix := "could not list Bazel versions in GCS bucket"
	if !strings.HasPrefix(err.Error(), expectedPrefix) {
		t.Fatalf("Expected error message that starts with '%s', but got '%v'", expectedPrefix, err)
	}
}

func TestResolveLatestVersion_NoFallbackIfGitHubForkIsDown(t *testing.T) {
	transport.AddResponse("https://api.github.com/repos/the_fork/bazel/releases", 500, "")

	_, err := resolveLatestVersion(tmpDir, "the_fork", 0)

	if err == nil {
		t.Fatal("Expected resolveLatestVersion() to fail.")
	}
	expectedSubstring := "github.com/the_fork/bazel"
	if !strings.Contains(err.Error(), expectedSubstring) {
		t.Fatalf("Expected error message that contains '%s', but got '%v'", expectedSubstring, err)
	}
}

type fakeTransport struct {
	responses map[string]*http.Response
}

func (ft *fakeTransport) AddResponse(url string, status int, body string) {
	ft.responses[url] = ft.createResponse(status, body)
}

func (ft *fakeTransport) createResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       ioutil.NopCloser(bytes.NewBufferString(body)),
	}
}

func (ft *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if resp, ok := ft.responses[req.URL.String()]; ok {
		return resp, nil
	}
	return ft.createResponse(http.StatusNotFound, ""), nil
}

func buildGCSResponseOrFail(t *testing.T, prefixes []string, items []interface{}) string {
	r := &gcsListResponse{
		Prefixes: prefixes,
		Items:    items,
	}
	if bytes, err := json.Marshal(r); err != nil {
		t.Fatalf("Could not build GCS json response: %v", err)
		return ""
	} else {
		return string(bytes)
	}
}
