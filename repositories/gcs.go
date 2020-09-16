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
func (gcs *GCSRepo) GetReleaseVersions(bazeliskHome string) ([]string, error) {
	return getVersionHistoryFromGCS(true)
}

func getVersionHistoryFromGCS(onlyFullReleases bool) ([]string, error) {
	prefixes, _, err := listDirectoriesInReleaseBucket("")
	if err != nil {
		return []string{}, fmt.Errorf("could not list Bazel versions in GCS bucket: %v", err)
	}

	available := getVersionsFromGCSPrefixes(prefixes)
	sorted := versions.GetInAscendingOrder(available)

	if onlyFullReleases && len(sorted) > 0 {
		latestVersion := sorted[len(sorted)-1]
		_, isRelease, err := listDirectoriesInReleaseBucket(latestVersion + "/release/")
		if err != nil {
			return []string{}, fmt.Errorf("could not list release candidates for latest release: %v", err)
		}
		if !isRelease {
			sorted = sorted[:len(sorted)-1]
		}
	}

	return sorted, nil
}

func listDirectoriesInReleaseBucket(prefix string) ([]string, bool, error) {
	url := "https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/"
	if prefix != "" {
		url = fmt.Sprintf("%s&prefix=%s", url, prefix)
	}
	content, err := httputil.ReadRemoteFile(url, "")
	if err != nil {
		return nil, false, fmt.Errorf("could not list GCS objects at %s: %v", url, err)
	}

	var response GcsListResponse
	if err := json.Unmarshal(content, &response); err != nil {
		return nil, false, fmt.Errorf("could not parse GCS index JSON: %v", err)
	}
	return response.Prefixes, len(response.Items) > 0, nil
}

func getVersionsFromGCSPrefixes(versions []string) []string {
	result := make([]string, len(versions))
	for i, v := range versions {
		result[i] = strings.ReplaceAll(v, "/", "")
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

// CandidateRepo

// GetCandidateVersions returns all versions of available release candidates in this repository.
func (gcs *GCSRepo) GetCandidateVersions(bazeliskHome string) ([]string, error) {
	available, err := getVersionHistoryFromGCS(false)
	if err != nil {
		return []string{}, err
	}

	if len(available) == 0 {
		return []string{}, errors.New("could not find any Bazel versions")
	}

	sorted := versions.GetInAscendingOrder(available)
	latestVersion := sorted[len(sorted)-1]

	// Append slash to match directories
	rcPrefixes, _, err := listDirectoriesInReleaseBucket(latestVersion + "/")
	if err != nil {
		return []string{}, fmt.Errorf("could not list release candidates for latest release: %v", err)
	}

	return getVersionsFromGCSPrefixes(rcPrefixes), nil
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
	content, err := httputil.ReadRemoteFile(lastGreenBaseURL+pathSuffix, "")
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
