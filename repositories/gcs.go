// Package repositories contains actual implementations of the repository interfaces defined in the `core` package.
// It currently supports Google Cloud Storage (GCS) for Bazel releases, release candidates and Bazel binaries built at arbitrary commits.
// Moreover, it supports GitHub for Bazel forks.
package repositories

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/bazelbuild/bazelisk/config"
	"github.com/bazelbuild/bazelisk/core"
	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/platforms"
	"github.com/bazelbuild/bazelisk/versions"
)

const (
	ltsBaseURL         = "https://releases.bazel.build"
	commitBaseURL      = "https://storage.googleapis.com/bazel-builds/artifacts"
	lastGreenCommitURL = "https://storage.googleapis.com/bazel-builds/last_green_commit/github.com/bazelbuild/bazel.git/publish-bazel-binaries"
)

// GCSRepo represents a Bazel repository on Google Cloud Storage that contains Bazel releases, release candidates and Bazel binaries built at arbitrary commits.
// It can return all available Bazel versions, as well as downloading a specific version.
type GCSRepo struct{}

// LTSRepo

// GetLTSVersions returns the versions of all available Bazel releases in this repository that match the given filter.
func (gcs *GCSRepo) GetLTSVersions(bazeliskHome string, opts *core.FilterOpts) ([]string, error) {
	history, err := getVersionHistoryFromGCS()
	if err != nil {
		return []string{}, err
	}
	matches, err := gcs.matchingVersions(history, opts)
	if err != nil {
		return []string{}, err
	}
	if len(matches) == 0 {
		var suffix string
		if opts.Track > 0 {
			suffix = fmt.Sprintf(" for track %d", opts.Track)
		}
		return []string{}, fmt.Errorf("could not find any LTS Bazel binaries%s", suffix)
	}
	return matches, nil
}

func getVersionHistoryFromGCS() ([]string, error) {
	prefixes, err := listDirectoriesInBucket("")
	if err != nil {
		return []string{}, fmt.Errorf("could not list Bazel versions in GCS bucket: %v", err)
	}

	available := getVersionsFromGCSPrefixes(prefixes)
	sorted := versions.GetInAscendingOrder(available)
	return sorted, nil
}

func listDirectoriesInBucket(prefix string) ([]string, error) {
	baseURL := "https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/"
	if prefix != "" {
		baseURL = fmt.Sprintf("%s&prefix=%s", baseURL, prefix)
	}

	var prefixes []string
	var nextPageToken = ""
	for {
		var url = baseURL
		if nextPageToken != "" {
			url = fmt.Sprintf("%s&pageToken=%s", baseURL, nextPageToken)
		}

		var content []byte
		var err error
		// Theoretically, this should always work, but we've seen transient
		// errors on Bazel CI, so we retry a few times to work around this.
		// https://github.com/bazelbuild/continuous-integration/issues/1627
		waitTime := 100 * time.Microsecond
		for attempt := 0; attempt < 5; attempt++ {
			content, _, err = httputil.ReadRemoteFile(url, "")
			if err == nil {
				break
			}
			time.Sleep(waitTime)
			waitTime *= 2
		}

		if err != nil {
			return nil, fmt.Errorf("could not list GCS objects at %s: %v", url, err)
		}

		var response GcsListResponse
		if err := json.Unmarshal(content, &response); err != nil {
			return nil, fmt.Errorf("could not parse GCS index JSON: %v", err)
		}

		prefixes = append(prefixes, response.Prefixes...)

		if response.NextPageToken == "" {
			break
		}
		nextPageToken = response.NextPageToken
	}
	return prefixes, nil
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

func getVersionsFromGCSPrefixes(versions []string) []string {
	result := make([]string, len(versions))
	for i, v := range versions {
		noSlashes := strings.Replace(v, "/", "", -1)
		result[i] = strings.TrimSuffix(noSlashes, "release")
	}
	return result
}

func (gcs *GCSRepo) matchingVersions(history []string, opts *core.FilterOpts) ([]string, error) {
	descendingMatches := make([]string, 0)
	// history is a list of base versions in ascending order (i.e. X.Y.Z, no rolling releases or candidates).
	for hpos := len(history) - 1; hpos >= 0; hpos-- {
		baseVersion := history[hpos]
		if opts.Track > 0 {
			track, err := getTrack(baseVersion)
			if err != nil {
				continue // Ignore invalid GCS entries for now
			}
			if track > opts.Track {
				continue
			} else if track < opts.Track {
				break
			}
		}

		// Append slash to match directories
		bucket := fmt.Sprintf("%s/", history[hpos])
		prefixes, err := listDirectoriesInBucket(bucket)
		if err != nil {
			return []string{}, fmt.Errorf("could not list LTS releases/candidates: %v", err)
		}

		// Ascending list of rc versions, followed by the release version (if it exists) and a rolling identifier (if there are rolling releases).
		versions := getVersionsFromGCSPrefixes(prefixes)
		for vpos := len(versions) - 1; vpos >= 0; vpos-- {
			curr := versions[vpos]
			if strings.Contains(curr, "rolling") || !opts.Filter(curr) {
				continue
			}

			descendingMatches = append(descendingMatches, curr)
			if len(descendingMatches) == opts.MaxResults {
				return descendingMatches, nil
			}
		}
	}
	return descendingMatches, nil
}

func getTrack(version string) (int, error) {
	major, _, _ := strings.Cut(version, ".")
	if val, err := strconv.Atoi(major); err == nil {
		return val, nil
	}
	return 0, fmt.Errorf("invalid version %q", version)
}

// DownloadLTS downloads the given Bazel LTS release (candidate) into the specified location and returns the absolute path.
func (gcs *GCSRepo) DownloadLTS(version, destDir, destFile string, config config.Config) (string, error) {
	srcFile, err := platforms.DetermineBazelFilename(version, true, config)
	if err != nil {
		return "", err
	}

	var baseVersion, folder string
	if strings.Contains(version, "rc") {
		versionComponents := strings.Split(version, "rc")
		baseVersion, folder = versionComponents[0], "rc"+versionComponents[1]
	} else {
		baseVersion, folder = version, "release"
	}

	url := fmt.Sprintf("%s/%s/%s/%s", ltsBaseURL, baseVersion, folder, srcFile)
	return httputil.DownloadBinary(url, destDir, destFile, config)
}

// CommitRepo

// GetLastGreenCommit returns the most recent commit at which a Bazel binary is successfully built.
func (gcs *GCSRepo) GetLastGreenCommit(bazeliskHome string) (string, error) {
	content, _, err := httputil.ReadRemoteFile(lastGreenCommitURL, "")
	if err != nil {
		return "", fmt.Errorf("could not determine last green commit: %v", err)
	}

	// Validate the content does look like a commit hash
	commit := strings.TrimSpace(string(content))
	if !versions.MatchCommitPattern(commit) {
		return "", fmt.Errorf("invalid commit hash: %s", commit)
	}

	return commit, nil
}

// DownloadAtCommit downloads a Bazel binary built at the given commit into the specified location and returns the absolute path.
func (gcs *GCSRepo) DownloadAtCommit(commit, destDir, destFile string, config config.Config) (string, error) {
	log.Printf("Using unreleased version at commit %s", commit)
	platform, err := platforms.GetPlatform()
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/%s/%s/bazel", commitBaseURL, platform, commit)
	return httputil.DownloadBinary(url, destDir, destFile, config)
}

// RollingRepo

// GetRollingVersions returns a list of all available rolling release versions for the newest release.
func (gcs *GCSRepo) GetRollingVersions(bazeliskHome string) ([]string, error) {
	history, err := getVersionHistoryFromGCS()
	if err != nil {
		return []string{}, err
	}

	newest := history[len(history)-1]
	versions, err := listDirectoriesInBucket(newest + "/rolling/")
	if err != nil {
		return []string{}, err
	}

	releases := make([]string, 0)
	for _, v := range versions {
		if !strings.Contains(v, "rc") {
			releases = append(releases, strings.Split(v, "/")[2])
		}
	}

	return releases, nil
}

// DownloadRolling downloads the given Bazel version into the specified location and returns the absolute path.
func (gcs *GCSRepo) DownloadRolling(version, destDir, destFile string, config config.Config) (string, error) {
	srcFile, err := platforms.DetermineBazelFilename(version, true, config)
	if err != nil {
		return "", err
	}

	releaseVersion := strings.Split(version, "-")[0]
	url := fmt.Sprintf("%s/%s/rolling/%s/%s", ltsBaseURL, releaseVersion, version, srcFile)
	return httputil.DownloadBinary(url, destDir, destFile, config)
}
