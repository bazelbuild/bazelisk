// Package repositories contains actual implementations of the repository interfaces defined in the `core` package.
// It currently supports Google Cloud Storage (GCS) for Bazel releases, release candidates and Bazel binaries built at arbitrary commits.
// Moreover, it supports GitHub for Bazel forks.
package repositories

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/platforms"
	"github.com/bazelbuild/bazelisk/versions"
)

const (
	candidateBaseURL    = "https://releases.bazel.build"
	nonCandidateBaseURL = "https://storage.googleapis.com/bazel-builds/artifacts"
	lastGreenBaseURL    = "https://storage.googleapis.com/bazel-untrusted-builds/last_green_commit/"
)

var (
	// key == includeDownstream
	lastGreenCommitPathSuffixes = map[bool]string{
		false: "github.com/bazelbuild/bazel.git/bazel-bazel",
		true:  "downstream_pipeline",
	}
)

// GCSRepo represents a Bazel repository on Google Cloud Storage that contains Bazel releases, release candidates and Bazel binaries built at arbitrary commits.
// It can return all available Bazel versions, as well as downloading a specific version.
type GCSRepo struct{}

// ReleaseRepo

// GetReleaseVersions returns the versions of all available Bazel releases in this repository.
func (gcs *GCSRepo) GetReleaseVersions(bazeliskHome string, lastN int) ([]string, error) {
	history, err := getVersionHistoryFromGCS()
	if err != nil {
		return []string{}, err
	}
	releases, err := gcs.removeCandidates(history, lastN)
	if err != nil {
		return []string{}, err
	}
	if len(releases) == 0 {
		return []string{}, errors.New("there are no releases available")
	}
	return releases, nil
}

func getVersionHistoryFromGCS() ([]string, error) {
	prefixes, _, err := listDirectoriesInReleaseBucket("")
	if err != nil {
		return []string{}, fmt.Errorf("could not list Bazel versions in GCS bucket: %v", err)
	}

	available := getVersionsFromGCSPrefixes(prefixes)
	sorted := versions.GetInAscendingOrder(available)
	return sorted, nil
}

func listDirectoriesInReleaseBucket(prefix string) ([]string, bool, error) {
	baseURL := "https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/"
	if prefix != "" {
		baseURL = fmt.Sprintf("%s&prefix=%s", baseURL, prefix)
	}

	var prefixes []string
	var isRelease = false
	var nextPageToken = ""
	for {
		var url = baseURL
		if nextPageToken != "" {
			url = fmt.Sprintf("%s&pageToken=%s", baseURL, nextPageToken)
		}
		content, _, err := httputil.ReadRemoteFile(url, "")
		if err != nil {
			return nil, false, fmt.Errorf("could not list GCS objects at %s: %v", url, err)
		}

		var response GcsListResponse
		if err := json.Unmarshal(content, &response); err != nil {
			return nil, false, fmt.Errorf("could not parse GCS index JSON: %v", err)
		}

		prefixes = append(prefixes, response.Prefixes...)
		isRelease = isRelease || len(response.Items) > 0

		if response.NextPageToken == "" {
			break
		}
		nextPageToken = response.NextPageToken
	}
	return prefixes, isRelease, nil
}

func getVersionsFromGCSPrefixes(versions []string) []string {
	result := make([]string, len(versions))
	for i, v := range versions {
		noSlashes := strings.Replace(v, "/", "", -1)
		result[i] = strings.TrimSuffix(noSlashes, "release")
	}
	return result
}

// GcsListResponse represents the result of listing the contents of a GCS bucket.
// Public for testing
type GcsListResponse struct {
	// Prefixes contains the available string prefixes.
	Prefixes []string `json:"prefixes"`

	// Items contains the names of available objects in the current GCS bucket.
	Items []interface{} `json:"items"`

	// Optional token when the result is paginated.
	NextPageToken string `json:"nextPageToken"`
}

// DownloadRelease downloads the given Bazel release into the specified location and returns the absolute path.
func (gcs *GCSRepo) DownloadRelease(version, destDir, destFile string) (string, error) {
	srcFile, err := platforms.DetermineBazelFilename(version, true)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/%s/release/%s", candidateBaseURL, version, srcFile)
	return httputil.DownloadBinary(url, destDir, destFile)
}

func (gcs *GCSRepo) removeCandidates(history []string, lastN int) ([]string, error) {
	var resolvedLimit int
	if lastN < 1 {
		resolvedLimit = len(history)
	} else {
		resolvedLimit = lastN
	}

	descendingReleases := make([]string, 0)
	for hpos := len(history) - 1; hpos >= 0 && len(descendingReleases) < resolvedLimit; hpos-- {
		latestVersion := history[hpos]
		_, isRelease, err := listDirectoriesInReleaseBucket(latestVersion + "/release/")
		if err != nil {
			return []string{}, fmt.Errorf("could not list available releases for %v: %v", latestVersion, err)
		}
		if isRelease {
			descendingReleases = append(descendingReleases, latestVersion)
		}
	}

	if lastN > 0 && len(descendingReleases) < lastN {
		return []string{}, fmt.Errorf("requested %d latest releases, but only found %d", lastN, len(descendingReleases))
	}
	return reverseInPlace(descendingReleases), nil
}

func reverseInPlace(values []string) []string {
	for i := 0; i < len(values)/2; i++ {
		j := len(values) - 1 - i
		values[i], values[j] = values[j], values[i]
	}
	return values
}

// CandidateRepo

// GetCandidateVersions returns all versions of available release candidates for the latest release in this repository.
func (gcs *GCSRepo) GetCandidateVersions(bazeliskHome string) ([]string, error) {
	history, err := getVersionHistoryFromGCS()
	if err != nil {
		return []string{}, err
	}

	if len(history) == 0 {
		return []string{}, errors.New("could not find any Bazel versions")
	}

	// Find most recent directory that contains any release candidates.
	// Typically it should be the last or second-to-last, depending on whether there are new rolling releases.
	for pos := len(history) - 1; pos >= 0; pos-- {
		// Append slash to match directories
		bucket := fmt.Sprintf("%s/", history[pos])
		rcPrefixes, _, err := listDirectoriesInReleaseBucket(bucket)
		if err != nil {
			return []string{}, fmt.Errorf("could not list release candidates for latest release: %v", err)
		}

		rcs := make([]string, 0)
		for _, v := range getVersionsFromGCSPrefixes(rcPrefixes) {
			// Remove full and rolling releases
			if strings.Contains(v, "rc") {
				rcs = append(rcs, v)
			}
		}
		if len(rcs) > 0 {
			return rcs, nil
		}
	}
	return nil, nil
}

// DownloadCandidate downloads the given release candidate into the specified location and returns the absolute path.
func (gcs *GCSRepo) DownloadCandidate(version, destDir, destFile string) (string, error) {
	if !strings.Contains(version, "rc") {
		return "", fmt.Errorf("'%s' does not refer to a release candidate", version)
	}

	srcFile, err := platforms.DetermineBazelFilename(version, true)
	if err != nil {
		return "", err
	}

	versionComponents := strings.Split(version, "rc")
	baseVersion := versionComponents[0]
	rcVersion := "rc" + versionComponents[1]
	url := fmt.Sprintf("%s/%s/%s/%s", candidateBaseURL, baseVersion, rcVersion, srcFile)
	return httputil.DownloadBinary(url, destDir, destFile)
}

// CommitRepo

// GetLastGreenCommit returns the most recent commit at which a Bazel binary passed a specific Bazel CI pipeline.
// If downstreamGreen is true, the pipeline is https://buildkite.com/bazel/bazel-at-head-plus-downstream, otherwise
// it's https://buildkite.com/bazel/bazel-bazel
func (gcs *GCSRepo) GetLastGreenCommit(bazeliskHome string, downstreamGreen bool) (string, error) {
	pathSuffix := lastGreenCommitPathSuffixes[downstreamGreen]
	content, _, err := httputil.ReadRemoteFile(lastGreenBaseURL+pathSuffix, "")
	if err != nil {
		return "", fmt.Errorf("could not determine last green commit: %v", err)
	}
	return strings.TrimSpace(string(content)), nil
}

// DownloadAtCommit downloads a Bazel binary built at the given commit into the specified location and returns the absolute path.
func (gcs *GCSRepo) DownloadAtCommit(commit, destDir, destFile string) (string, error) {
	log.Printf("Using unreleased version at commit %s", commit)
	url := fmt.Sprintf("%s/%s/%s/bazel", nonCandidateBaseURL, platforms.GetPlatform(), commit)
	return httputil.DownloadBinary(url, destDir, destFile)
}
