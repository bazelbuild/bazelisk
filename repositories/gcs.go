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
	verificationKey     = `
-----BEGIN PGP PUBLIC KEY BLOCK-----

mQINBFdEmzkBEACzj8tMYUau9oFZWNDytcQWazEO6LrTTtdQ98d3JcnVyrpT16yg
I/QfGXA8LuDdKYpUDNjehLtBL3IZp4xe375Jh8v2IA2iQ5RXGN+lgKJ6rNwm15Kr
qYeCZlU9uQVpZuhKLXsWK6PleyQHjslNUN/HtykIlmMz4Nnl3orT7lMI5rsGCmk0
1Kth0DFh8SD9Vn2G4huddwxM8/tYj1QmWPCTgybATNuZ0L60INH8v6+J2jJzViVc
NRnR7mpouGmRy/rcr6eY9QieOwDou116TrVRFfcBRhocCI5b6uCRuhaqZ6Qs28Bx
4t5JVksXJ7fJoTy2B2s/rPx/8j4MDVEdU8b686ZDHbKYjaYBYEfBqePXScp8ndul
XWwS2lcedPihOUl6oQQYy59inWIpxi0agm0MXJAF1Bc3ToSQdHw/p0Y21kYxE2pg
EaUeElVccec5poAaHSPprUeej9bD9oIC4sMCsLs7eCQx2iP+cR7CItz6GQtuZrvS
PnKju1SKl5iwzfDQGpi6u6UAMFmc53EaH05naYDAigCueZ+/2rIaY358bECK6/VR
kyrBqpeq6VkWUeOkt03VqoPzrw4gEzRvfRtLj+D2j/pZCH3vyMYHzbaaXBv6AT0e
RmgtGo9I9BYqKSWlGEF0D+CQ3uZfOyovvrbYqNaHynFBtrx/ZkM82gMA5QARAQAB
tEdCYXplbCBEZXZlbG9wZXIgKEJhemVsIEFQVCByZXBvc2l0b3J5IGtleSkgPGJh
emVsLWRldkBnb29nbGVncm91cHMuY29tPokCVQQTAQgAPwIbAwYLCQgHAwIGFQgC
CQoLBBYCAwECHgECF4AWIQRxodDvz+tigf0EN8k9WRm0SEV+4AUCXsoWGgUJC0fh
4QAKCRA9WRm0SEV+4NDCD/9c5rhZREBlikdi5QYRq1YOkwzJLXFoVe0FonEwMuWK
fQzT/rIwyh14tssptU5+eXwTEXL0ZDskgzvrFSpzjQZzcSG/gzNCATNfrZpC2nfE
SxMKOeIwQedn26YIHCI8s9tEQ7BSvfBfJgqfIo3IURhmfzNMj+qszca+3IDYAlAy
8lxUVbJcIQ0apnAdnIadtydzca56mMN7ma+btddaWLpAdyfUvQ/Zsx3TYYLF7inQ
km0JpzISN0fGngzGNDGNmtHNhCdSpyfkr+7fvpbKAYkSH7uZ1AIPDyHdLIwDQnX2
kbLRkxKncKGSDhUSdlJTl0x36cU+xmgO15FFdOyk3BUfrlfDrgXIBjeX8KNh9TV6
HgFFR/mNONoJ93ZvZQNO2s1gbPZJe3VJ1Q5PMLW1sdl8q8JthBwT/5TJ1k8E5VYj
jAc8dl+RAALxqj+eo5xI45o1FdV5s1aGDjbwFoCIhGCy2zaog1q5wnhmEptAAD0S
TVbJSpwNiLlPIcGVaCjXp8Ow3SzOGTRKIjFTO/I6FiSJOpgfri07clXmnb4ETjou
mUdglg8/8nQ120zHEOqoSzzIbTNUDjNZY8SuY6Ig3/ObQ/JAFS0i6h74KLfXUZzn
uETY7KURLdyPAhL37Hb9FDhvkJCUO/l6eqDh9jk1JjB7Cvb7hEvnbvDrr2hWNAL7
RrkCDQRXRJs5ARAA55/1VBlDpV/ElUyLmRyPCz/V+msHdinyw4Mv5DJQupuZwlMy
vxPPzc7GmsIfk1zuOzDWirNs22r43ak6dsAvpcU+iVBi46MqUcbNtC+kfxlKiToD
PCs82rdfCgHT7XYDzrCWlqNQ9++BqM2OYRIxyEucizeofWPlrJUgKvu8fWLVZ6bY
n4L/PqAhobhuSjRcoB5Tp81hGa4cscKIGIqhymfnguaY8viJ83tHPUqQJoApNPy8
q1pWHSDV6zBv71beqV2b6cBzp7VqNYOIuqE6ZNBFWuCG3zRc9ia2/bHxx2TGAQJt
PpPzitm0xkB3GGN06YnnSCE+f2j+7F0IO6uFlSy7ho0PoSFbDgR91kJK3S0ZBZx4
H21cIpWWBzf9Nd1M4H3O7KhnGSZDq6+tXZ9/F/ZUvCZHpQlJewDPY9315Ymacf5C
Zk8xeE5UUIxFMdOxF8B7Itb6rbFWv+tzWdX/0/M8/b0ZJhVvngWzuh/agdS4E5an
f7ahGWM96jPRIQEb9DRN2YGp9hOiX2sZqkhxE5zWqD2gdXp2ZAxMCTHf4ijzOVsO
nde7b5BqC0JL73gNwf1iOHyCAzqGiFfah8/odBTDhMsdVMsjSIxzcwlwRnzy+hBs
dYpP19ieJCMoERJTbUgSspPdhY/Y4ChzlFHjiAKYT6vXiYcKS04stCtHqwEAEQEA
AYkCPAQYAQgAJgIbDBYhBHGh0O/P62KB/QQ3yT1ZGbRIRX7gBQJeyhYlBQkLR+Hs
AAoJED1ZGbRIRX7g3Y8P/iuOAHmyCMeSELvUs9ZvLYJKGzmz67R8fJSmgst/Bs3p
dWCAjGE56M6UgZzHXK+fBRWFPDOXT64XNq0UIG7tThthwe4Gdvg/5rWG61Pe/vCZ
2FkMAlEMkuufZYMcw9jItHMKLcYyW/jtN9EzCX+vM6SZlu4o8la5rCIBEaiKfzft
a/dRMjW+RqQnU31NQCDAy3zoGUCQumJtv3GVbMYHIrRZua2yyNo9Iborh2SVdBbK
v9WJKH4JcCHd0/XDGdys6EXeATIIRxchumkmxpIg87OhsC0n5yuH1FnFIFQEjbYX
bb46F7ZFT+8Tov+lgMEw4CZmps4uvvZlKbIH4Zi/ULiobwvm2ad3nejWICmGmHYz
ro6t08hdcY6GnOzCpDwx9yHechMCkU3KEE98nb/CxcmA4VzDHudTJe7o0OyaSarh
6D5WcXf7D9FfcKmUD9xaCsfXh66OCksMVGE1JctrO1wQTF2jTdTUq7mmi30tlM+o
JjVk65OSOd4JYol8auzE4oXOfsNzXbyvj7WzM1v5m7C45jOL+Ly7I3IUzZNfF41J
AMmSd73EOoR9YH4qTrL3jx69Ekf7ww70Qea5enLE8xUgQfGTOaEHxkFcEovmzv54
6IVe083iK8alXD/9OUTaDY9NwMnOn1K1aU2XOfliGGLgwwaHg+wVFh5rZIHsDl7v
=Embu
-----END PGP PUBLIC KEY BLOCK-----
`
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
	return httputil.DownloadBinary(url, url+".sig", verificationKey, destDir, destFile)
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
	return httputil.DownloadBinary(url, url+".sig", verificationKey, destDir, destFile)
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
	return httputil.DownloadBinary(url, "", "", destDir, destFile)
}
