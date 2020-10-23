package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/bazelbuild/bazelisk/core"
	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/repositories"
	"github.com/bazelbuild/bazelisk/versions"
)

var (
	transport = &fakeTransport{responses: make(map[string]*http.Response)}
	tmpDir    = ""
)

func init() {
	httputil.DefaultTransport = transport
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

func TestResolveLatestRcVersion(t *testing.T) {
	s := setUp(t)
	s.AddVersion("4.0.0", false)
	s.AddVersion("10.0.0", false)
	s.AddVersion("11.0.0", true)
	s.AddVersion("11.11.0", false, 1, 2)
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(nil, gcs, nil, nil, false)
	version, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "last_rc")

	if err != nil {
		t.Fatalf("Version resolution failed unexpectedly: %v", err)
	}
	expectedRC := "11.11.0rc2"
	if version != expectedRC {
		t.Fatalf("Expected version %s, but got %s", expectedRC, version)
	}
}

func TestResolveLatestRcVersion_WithFullRelease(t *testing.T) {
	s := setUp(t)
	s.AddVersion("4.0.0", true, 1, 2, 3)
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(nil, gcs, nil, nil, false)
	version, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "last_rc")

	if err != nil {
		t.Fatalf("Version resolution failed unexpectedly: %v", err)
	}
	expectedRC := "4.0.0rc3"
	if version != expectedRC {
		t.Fatalf("Expected version %s, but got %s", expectedRC, version)
	}
}

func TestResolveLatestVersion_TwoLatestVersionsDoNotHaveAReleaseYet(t *testing.T) {
	s := setUp(t)
	s.AddVersion("4.0.0", true)
	s.AddVersion("5.0.0", false)
	s.AddVersion("6.0.0", false)
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	version, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "latest")

	if err != nil {
		t.Fatalf("Version resolution failed unexpectedly: %v", err)
	}
	expectedVersion := "4.0.0"
	if version != expectedVersion {
		t.Fatalf("Expected version %s, but got %s", expectedVersion, version)
	}
}

func TestResolveLatestVersion_FilterReleaseCandidates(t *testing.T) {
	s := setUp(t)
	s.AddVersion("3.0.0", true)
	s.AddVersion("4.0.0", false)
	s.AddVersion("5.0.0", false)
	s.AddVersion("6.0.0", true)
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	version, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "latest-1")

	if err != nil {
		t.Fatalf("Version resolution failed unexpectedly: %v", err)
	}
	expectedVersion := "3.0.0"
	if version != expectedVersion {
		t.Fatalf("Expected version %s, but got %s", expectedVersion, version)
	}
}

func TestResolveLatestVersion_ShouldFailIfNotEnoughReleases(t *testing.T) {
	s := setUp(t)
	s.AddVersion("3.0.0", true)
	s.AddVersion("4.0.0", false)
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	_, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "latest-1")

	if err == nil {
		t.Fatal("Expected ResolveVersion() to fail.")
	}
	expectedError := "unable to determine latest version: requested 2 latest releases, but only found 1"
	if err.Error() != expectedError {
		t.Fatalf("Expected error message '%s', but got '%v'", expectedError, err)
	}
}

func TestResolveLatestVersion_GCSIsDown(t *testing.T) {
	setUp(t).WithError().Finish()

	transport.AddResponse("https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/", 500, "")

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	_, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "latest")

	if err == nil {
		t.Fatal("Expected resolveLatestVersion() to fail.")
	}
	expectedPrefix := "unable to determine latest version: could not list Bazel versions in GCS bucket"
	if !strings.HasPrefix(err.Error(), expectedPrefix) {
		t.Fatalf("Expected error message that starts with '%s', but got '%v'", expectedPrefix, err)
	}
}

func TestResolveLatestVersion_GitHubIsDown(t *testing.T) {
	transport.AddResponse("https://api.github.com/repos/bazelbuild/bazel/releases", 500, "")

	gh := repositories.CreateGitHubRepo("test_token")
	repos := core.CreateRepositories(nil, nil, gh, nil, false)

	_, _, err := repos.ResolveVersion(tmpDir, "some_fork", "latest")

	if err == nil {
		t.Fatal("Expected resolveLatestVersion() to fail.")
	}
	expectedPrefix := "unable to determine latest version: unable to dermine 'some_fork' releases: could not download list of Bazel releases from github.com/some_fork"
	if !strings.HasPrefix(err.Error(), expectedPrefix) {
		t.Fatalf("Expected error message that starts with '%s', but got '%v'", expectedPrefix, err)
	}
}

type gcsSetup struct {
	baseURL         string
	versionPrefixes []string
	status          int
	test            *testing.T
}

func (g *gcsSetup) AddVersion(version string, hasRelease bool, rcs ...int) *gcsSetup {
	g.versionPrefixes = append(g.versionPrefixes, fmt.Sprintf("%s/", version))
	prefixes := make([]string, 0)
	for _, rc := range rcs {
		prefix := fmt.Sprintf("%s/rc%d/", version, rc)
		prefixes = append(prefixes, prefix)
		g.addURL(prefix, false)
	}

	// The /release/ URLs have to exist, even if there is no release. In this case GCS returns no items, though.
	releasePrefix := fmt.Sprintf("%s/release/", version)
	g.addURL(releasePrefix, hasRelease)
	if hasRelease {
		prefixes = append(prefixes, releasePrefix)
	}

	g.addURL(fmt.Sprintf("%s/", version), false, prefixes...)
	return g
}

func (g *gcsSetup) addURL(prefix string, containsItem bool, childPrefixes ...string) {
	items := make([]interface{}, 0)
	if containsItem {
		items = append(items, "this_is_a_release")
	}
	resp := buildGCSResponseOrFail(g.test, childPrefixes, items)
	transport.AddResponse(fmt.Sprintf("%s&prefix=%s", g.baseURL, prefix), 200, resp)
}

func setUp(t *testing.T) *gcsSetup {
	return &gcsSetup{
		baseURL:         "https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/",
		status:          200,
		versionPrefixes: make([]string, 0),
		test:            t,
	}
}

func (g *gcsSetup) WithError() *gcsSetup {
	g.status = 500
	return g
}

func (g *gcsSetup) Finish() {
	// TODO: sort and deduplicate versionPrefixes
	listBody := buildGCSResponseOrFail(g.test, g.versionPrefixes, []interface{}{})
	transport.AddResponse(g.baseURL, g.status, listBody)
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
	r := &repositories.GcsListResponse{
		Prefixes: prefixes,
		Items:    items,
	}
	byteValue, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Could not build GCS json response: %v", err)
		return ""
	}
	return string(byteValue)
}
