package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/bazelbuild/bazelisk/config"
	"github.com/bazelbuild/bazelisk/core"
	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/repositories"
	"github.com/bazelbuild/bazelisk/versions"
)

const (
	rollingReleaseIdentifier = "rolling"
)

var (
	tmpDir = ""
)

func TestGetInAscendingOrder(t *testing.T) {
	input := []string{"7.0.1rc2", "6.1.0", "6.0.1", "10.11.12", "6.0.0", "7.0.1", "6.0.0rc2", "7.0.1rc1", "10.11.12rc1", "6.0.0rc1"}
	want := []string{"6.0.0rc1", "6.0.0rc2", "6.0.0", "6.0.1", "6.1.0", "7.0.1rc1", "7.0.1rc2", "7.0.1", "10.11.12rc1", "10.11.12"}

	got := versions.GetInAscendingOrder(input)
	if !slices.Equal(got, want) {
		t.Errorf("GetInAscendingOrder(): got %s, want %s", got, want)
	}
}

func TestMain(m *testing.M) {
	var err error
	tmpDir, err = os.MkdirTemp("", "version_test")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	code := m.Run()
	os.Exit(code)
}

func TestResolveVersion(t *testing.T) {
	s := setUp(t)
	s.AddVersion("4.0.0", false, nil, nil)
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	version, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "4.0.0", config.Null())

	if err != nil {
		t.Fatalf("Version resolution failed unexpectedly: %v", err)
	}
	expectedRC := "4.0.0"
	if version != expectedRC {
		t.Fatalf("Expected version %s, but got %s", expectedRC, version)
	}
}

func TestResolvePatchVersion(t *testing.T) {
	s := setUp(t)
	s.AddVersion("4.0.0-patch1", false, nil, nil)
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	version, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "4.0.0-patch1", config.Null())

	if err != nil {
		t.Fatalf("Version resolution failed unexpectedly: %v", err)
	}
	expectedRC := "4.0.0-patch1"
	if version != expectedRC {
		t.Fatalf("Expected version %s, but got %s", expectedRC, version)
	}
}

func TestResolveLatestRcVersion(t *testing.T) {
	s := setUp(t)
	s.AddVersion("4.0.0", false, nil, nil)
	s.AddVersion("10.0.0", false, nil, nil)
	s.AddVersion("11.0.0", true, nil, nil)
	s.AddVersion("11.11.0", false, []int{1, 2}, nil)
	s.AddVersion("12.0.0", false, nil, []string{"12.0.0-pre.20210504.1"})
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	version, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "last_rc", config.Null())

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
	s.AddVersion("4.0.0", true, []int{1, 2, 3}, nil)
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	version, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "last_rc", config.Null())

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
	s.AddVersion("4.0.0", true, nil, nil)
	s.AddVersion("5.0.0", false, nil, nil)
	s.AddVersion("6.0.0", false, nil, nil)
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	version, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "latest", config.Null())

	if err != nil {
		t.Fatalf("Version resolution failed unexpectedly: %v", err)
	}
	expectedVersion := "4.0.0"
	if version != expectedVersion {
		t.Fatalf("Expected version %s, but got %s", expectedVersion, version)
	}
}

func TestResolveLatestVersion_ShouldOnlyReturnStableReleases(t *testing.T) {
	s := setUp(t)
	s.AddVersion("3.0.0", true, []int{1}, nil)
	s.AddVersion("4.0.0", false, nil, nil)
	s.AddVersion("5.0.0", false, nil, nil)
	s.AddVersion("6.0.0", true, []int{1, 2}, nil)
	s.AddVersion("7.0.0", false, nil, []string{"12.0.0-pre.20210504.1"})
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	version, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "latest-1", config.Null())

	if err != nil {
		t.Fatalf("Version resolution failed unexpectedly: %v", err)
	}
	expectedVersion := "3.0.0"
	if version != expectedVersion {
		t.Fatalf("Expected version %s, but got %s", expectedVersion, version)
	}
}

func TestResolveLatestAvoidsUnnecessaryRequests(t *testing.T) {
	tests := []struct{
		name string
		specifiedVersion string
		wantVersion string
	} {
		{
			name: "Release",
			specifiedVersion: "latest",
			wantVersion: "7.0.0",
		},
		{
			name: "Candidate",
			specifiedVersion: "last_rc",
			wantVersion: "7.0.0rc1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T){
		s := setUp(t)
		s.AddVersion("4.0.0", true, []int{1}, nil)
		s.AddVersion("5.0.0", true, []int{1}, nil)
		s.AddVersion("6.0.0", true, []int{1}, nil)
		s.AddVersion("7.0.0", true, []int{1}, nil)
		s.AddVersion("8.0.0", false, nil, nil)
		s.Finish()

		gcs := &repositories.GCSRepo{}
		repos := core.CreateRepositories(gcs, nil, nil, nil, false)
		gotVersion, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, test.specifiedVersion, config.Null())

		if err != nil {
			t.Fatalf("Version resolution failed unexpectedly: %v", err)
		}
		if gotVersion != test.wantVersion {
			t.Errorf("Expected version %s, but got %s", test.wantVersion, gotVersion)
		}
		/*
		We expect three requests:
		- One to get a list of all Bazel version tracks
		- One to check for the existence of the 8.* release (candidates), which returns no results
		- One to check for the existence of the 7.* release (candidates), which returns a match
		*/
		wantRequests := 3
		if gotRequests := len(s.Transport.RequestedURLs); gotRequests != wantRequests {
			t.Errorf("Expected exactly %d requests (one for the top-level, one for 8.0.0, one for 7.0.0), but got %d:\n%s", wantRequests, gotRequests, strings.Join(s.Transport.RequestedURLs, "\n"))
		}
	})
	}
}

func TestResolveLatestVersion_ShouldFailIfNotEnoughReleases(t *testing.T) {
	s := setUp(t)
	s.AddVersion("3.0.0", true, nil, nil)
	s.AddVersion("4.0.0", false, nil, nil)
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	_, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "latest-1", config.Null())

	if err == nil {
		t.Fatal("Expected ResolveVersion() to fail.")
	}
	expectedError := "cannot resolve version \"latest-1\": There are not enough matching Bazel releases (1)"
	if err.Error() != expectedError {
		t.Fatalf("Expected error message %q, but got '%v'", expectedError, err)
	}
}

func TestResolveLatestVersion_GCSIsDown(t *testing.T) {
	g := setUp(t).WithError().Finish()
	g.Transport.AddResponse("https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/", 500, "", nil)

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(gcs, nil, nil, nil, false)
	_, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, "latest", config.Null())

	if err == nil {
		t.Fatal("Expected resolveLatestVersion() to fail.")
	}
	expectedPrefix := "unable to determine latest version: could not list Bazel versions in GCS bucket"
	if !strings.HasPrefix(err.Error(), expectedPrefix) {
		t.Fatalf("Expected error message that starts with %q, but got '%v'", expectedPrefix, err)
	}
}

func TestResolveLatestVersion_GitHubIsDown(t *testing.T) {
	transport := installTransport()
	transport.AddResponse("https://api.github.com/repos/bazelbuild/bazel/releases", 500, "", nil)

	gh := repositories.CreateGitHubRepo("test_token")
	repos := core.CreateRepositories(nil, gh, nil, nil, false)

	_, _, err := repos.ResolveVersion(tmpDir, "some_fork", "latest", config.Null())

	if err == nil {
		t.Fatal("Expected resolveLatestVersion() to fail.")
	}
	expectedPrefix := "unable to determine latest version: unable to determine 'some_fork' releases: could not download list of Bazel releases from github.com/some_fork"
	if !strings.HasPrefix(err.Error(), expectedPrefix) {
		t.Fatalf("Expected error message that starts with %q, but got '%v'", expectedPrefix, err)
	}
}

func TestAcceptRollingReleaseName(t *testing.T) {
	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(nil, nil, nil, gcs, false)

	for _, version := range []string{"10.0.0-pre.20201103.4", "10.0.0-pre.20201103.4.2"} {
		resolvedVersion, _, err := repos.ResolveVersion(tmpDir, "", version, config.Null())

		if err != nil {
			t.Fatalf("ResolveVersion(%q, \"\", %q): expected no error, but got %v", tmpDir, version, err)
		}

		if resolvedVersion != version {
			t.Fatalf("ResolveVersion(%q, \"\", %q) = %v, but expected %v", tmpDir, version, resolvedVersion, version)
		}
	}
}

func TestResolveLatestRollingRelease(t *testing.T) {
	s := setUp(t)
	s.AddVersion("11.0.0", false, nil, []string{"11.0.0/rolling/11.0.0-pre.20210503.1"})
	s.AddVersion("12.0.0", false, nil, []string{"12.0.0/rolling/12.0.0-pre.20210504.1"})
	s.Finish()

	gcs := &repositories.GCSRepo{}
	repos := core.CreateRepositories(nil, nil, nil, gcs, false)

	version, _, err := repos.ResolveVersion(tmpDir, "", rollingReleaseIdentifier, config.Null())

	if err != nil {
		t.Fatalf("ResolveVersion(%q, \"\", %q): expected no error, but got %v", tmpDir, rollingReleaseIdentifier, err)
	}

	want := "12.0.0-pre.20210504.1"
	if version != want {
		t.Fatalf("ResolveVersion(%q, \"\", %q) = %v, but expected %v", tmpDir, rollingReleaseIdentifier, version, want)
	}
}

func TestAcceptTrackBasedReleaseVersions(t *testing.T) {
	tests := []struct {
		name             string
		requestedVersion string
		releaseExists    bool
		wantVersion      string
	}{
		{
			name:             "Floating_ReleaseExists",
			requestedVersion: "4.x",
			releaseExists:    true,
			wantVersion:      "4.2.1",
		},
		{
			name:             "Floating_NoRelease",
			requestedVersion: "4.x",
			releaseExists:    false,
			wantVersion:      "4.2.0",
		},
		{
			name:             "Wildcard_ReleaseExists",
			requestedVersion: "4.*",
			releaseExists:    true,
			wantVersion:      "4.2.1",
		},
		{
			name:             "Wildcard_NoRelease",
			requestedVersion: "4.*",
			releaseExists:    false,
			wantVersion:      "4.2.1rc3",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := setUp(t)
			s.AddVersion("4.0.0", true, nil, []string{"4.0.0-pre.20210504.1"})
			s.AddVersion("4.1.0", true, nil, nil)
			s.AddVersion("4.2.0", true, nil, nil)
			s.AddVersion("4.2.1", test.releaseExists, []int{1, 2, 3}, nil)
			s.AddVersion("5.0.0", true, nil, nil)
			s.Finish()

			gcs := &repositories.GCSRepo{}
			repos := core.CreateRepositories(gcs, nil, nil, nil, false)
			version, _, err := repos.ResolveVersion(tmpDir, versions.BazelUpstream, test.requestedVersion, config.Null())

			if err != nil {
				t.Fatalf("Version resolution failed unexpectedly: %v", err)
			}
			if version != test.wantVersion {
				t.Fatalf("Expected version %s, but got %s", test.wantVersion, version)
			}
		})
	}
}

type gcsSetup struct {
	baseURL         string
	versionPrefixes []string
	status          int
	test            *testing.T
	Transport       *httputil.FakeTransport
}

func (g *gcsSetup) AddVersion(version string, hasRelease bool, rcs []int, rolling []string) *gcsSetup {
	g.versionPrefixes = append(g.versionPrefixes, fmt.Sprintf("%s/", version))
	prefixes := make([]string, 0)

	register := func(subDir string) {
		path := fmt.Sprintf("%s/%s/", version, subDir)
		prefixes = append(prefixes, path)
		g.addURL(path, false)
	}

	for _, rc := range rcs {
		register(fmt.Sprintf("rc%d", rc))
	}

	g.addURL(fmt.Sprintf("%s/rolling/", version), false, rolling...)

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
	g.Transport.AddResponse(fmt.Sprintf("%s&prefix=%s", g.baseURL, prefix), 200, resp, nil)
}

func setUp(t *testing.T) *gcsSetup {
	return &gcsSetup{
		baseURL:         "https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/",
		status:          200,
		versionPrefixes: make([]string, 0),
		test:            t,
		Transport:       installTransport(),
	}
}

func installTransport() *httputil.FakeTransport {
	ft := httputil.NewFakeTransport()
	httputil.DefaultTransport = ft
	return ft
}

func (g *gcsSetup) WithError() *gcsSetup {
	g.status = 500
	return g
}

func (g *gcsSetup) Finish() *gcsSetup {
	// TODO: sort and deduplicate versionPrefixes
	listBody := buildGCSResponseOrFail(g.test, g.versionPrefixes, []interface{}{})
	g.Transport.AddResponse(g.baseURL, g.status, listBody, nil)
	return g
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
