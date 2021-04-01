package core

import (
	"errors"
	"fmt"

	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/platforms"
	"github.com/bazelbuild/bazelisk/versions"
)

const (
	// BaseURLEnv is the name of the environment variable that stores the base URL for downloads.
	BaseURLEnv = "BAZELISK_BASE_URL"
)

// DownloadFunc downloads a specific Bazel binary to the given location and returns the absolute path.
type DownloadFunc func(destDir, destFile string) (string, error)

// ReleaseRepo represents a repository that stores LTS Bazel releases.
type ReleaseRepo interface {
	// GetReleaseVersions returns a list of all available release versions. If lastN is smaller than 1, all available versions are being returned.
	GetReleaseVersions(bazeliskHome string, lastN int) ([]string, error)

	// DownloadRelease downloads the given Bazel version into the specified location and returns the absolute path.
	DownloadRelease(version, destDir, destFile string) (string, error)
}

// CandidateRepo represents a repository that stores Bazel release candidates.
type CandidateRepo interface {
	// GetCandidateVersions returns the versions of all available release candidates.
	GetCandidateVersions(bazeliskHome string) ([]string, error)

	// DownloadCandidate downloads the given Bazel release candidate into the specified location and returns the absolute path.
	DownloadCandidate(version, destDir, destFile string) (string, error)
}

// ForkRepo represents a repository that stores a fork of Bazel (releases).
type ForkRepo interface {
	// GetVersions returns the versions of all available Bazel binaries in the given fork.
	GetVersions(bazeliskHome, fork string) ([]string, error)

	// DownloadVersion downloads the given Bazel binary from the specified fork into the given location and returns the absolute path.
	DownloadVersion(fork, version, destDir, destFile string) (string, error)
}

// CommitRepo represents a repository that stores Bazel binaries built at specific commits.
// It can also return the hashes of the most recent commits that passed Bazel CI pipelines successfully.
type CommitRepo interface {
	// GetLastGreenCommit returns the most recent commit at which a Bazel binary passed a specific Bazel CI pipeline.
	// If downstreamGreen is true, the pipeline is https://buildkite.com/bazel/bazel-at-head-plus-downstream, otherwise
	// it's https://buildkite.com/bazel/bazel-bazel
	GetLastGreenCommit(bazeliskHome string, downstreamGreen bool) (string, error)

	// DownloadAtCommit downloads a Bazel binary built at the given commit into the specified location and returns the absolute path.
	DownloadAtCommit(commit, destDir, destFile string) (string, error)
}

// RollingRepo represents a repository that stores rolling Bazel releases.
type RollingRepo interface {
	// GetRollingVersions returns a list of all available rolling release versions.
	GetRollingVersions(bazeliskHome string) ([]string, error)

	// DownloadRolling downloads the given Bazel version into the specified location and returns the absolute path.
	DownloadRolling(version, destDir, destFile string) (string, error)
}

// Repositories offers access to different types of Bazel repositories, mainly for finding and downloading the correct version of Bazel.
type Repositories struct {
	Releases        ReleaseRepo
	Candidates      CandidateRepo
	Fork            ForkRepo
	Commits         CommitRepo
	Rolling         RollingRepo
	supportsBaseURL bool
}

// ResolveVersion resolves a potentially relative Bazel version string such as "latest" to an absolute version identifier, and returns this identifier alongside a function to download said version.
func (r *Repositories) ResolveVersion(bazeliskHome, fork, version string) (string, DownloadFunc, error) {
	vi, err := versions.Parse(fork, version)
	if err != nil {
		return "", nil, err
	}

	if vi.IsFork {
		return r.resolveFork(bazeliskHome, vi)
	} else if vi.IsRelease {
		return r.resolveRelease(bazeliskHome, vi)
	} else if vi.IsCandidate {
		return r.resolveCandidate(bazeliskHome, vi)
	} else if vi.IsCommit {
		return r.resolveCommit(bazeliskHome, vi)
	} else if vi.IsRolling {
		return r.resolveRolling(bazeliskHome, vi)
	}

	return "", nil, fmt.Errorf("Unsupported version identifier '%s'", version)
}

func (r *Repositories) resolveFork(bazeliskHome string, vi *versions.Info) (string, DownloadFunc, error) {
	if vi.IsRelative && (vi.IsCandidate || vi.IsCommit) {
		return "", nil, errors.New("forks do not support last_rc, last_green and last_downstream_green")
	}
	lister := func(bazeliskHome string) ([]string, error) {
		return r.Fork.GetVersions(bazeliskHome, vi.Fork)
	}
	version, err := resolvePotentiallyRelativeVersion(bazeliskHome, lister, vi)
	if err != nil {
		return "", nil, err
	}
	downloader := func(destDir, destFile string) (string, error) {
		return r.Fork.DownloadVersion(vi.Fork, version, destDir, destFile)
	}
	return version, downloader, nil
}

func (r *Repositories) resolveRelease(bazeliskHome string, vi *versions.Info) (string, DownloadFunc, error) {
	lister := func(bazeliskHome string) ([]string, error) {
		return r.Releases.GetReleaseVersions(bazeliskHome, vi.LatestOffset+1)
	}
	version, err := resolvePotentiallyRelativeVersion(bazeliskHome, lister, vi)
	if err != nil {
		return "", nil, err
	}
	downloader := func(destDir, destFile string) (string, error) {
		return r.Releases.DownloadRelease(version, destDir, destFile)
	}
	return version, downloader, nil
}

func (r *Repositories) resolveCandidate(bazeliskHome string, vi *versions.Info) (string, DownloadFunc, error) {
	version, err := resolvePotentiallyRelativeVersion(bazeliskHome, r.Candidates.GetCandidateVersions, vi)
	if err != nil {
		return "", nil, err
	}
	downloader := func(destDir, destFile string) (string, error) {
		return r.Candidates.DownloadCandidate(version, destDir, destFile)
	}
	return version, downloader, nil
}

func (r *Repositories) resolveCommit(bazeliskHome string, vi *versions.Info) (string, DownloadFunc, error) {
	version := vi.Value
	if vi.IsRelative {
		var err error
		version, err = r.Commits.GetLastGreenCommit(bazeliskHome, vi.IsDownstream)
		if err != nil {
			return "", nil, fmt.Errorf("cannot resolve last green commit: %v", err)
		}
	}
	downloader := func(destDir, destFile string) (string, error) {
		return r.Commits.DownloadAtCommit(version, destDir, destFile)
	}
	return version, downloader, nil
}

func (r *Repositories) resolveRolling(bazeliskHome string, vi *versions.Info) (string, DownloadFunc, error) {
	lister := func(bazeliskHome string) ([]string, error) {
		return r.Rolling.GetRollingVersions(bazeliskHome)
	}
	version, err := resolvePotentiallyRelativeVersion(bazeliskHome, lister, vi)
	if err != nil {
		return "", nil, err
	}
	downloader := func(destDir, destFile string) (string, error) {
		return r.Rolling.DownloadRolling(version, destDir, destFile)
	}
	return version, downloader, nil
}

type listVersionsFunc func(bazeliskHome string) ([]string, error)

func resolvePotentiallyRelativeVersion(bazeliskHome string, lister listVersionsFunc, vi *versions.Info) (string, error) {
	if !vi.IsRelative {
		return vi.Value, nil
	}

	available, err := lister(bazeliskHome)
	if err != nil {
		return "", fmt.Errorf("unable to determine latest version: %v", err)
	}
	index := len(available) - 1 - vi.LatestOffset
	if index < 0 {
		return "", fmt.Errorf("cannot resolve version \"%s\": There are only %d Bazel versions", vi.Value, len(available))
	}
	sorted := versions.GetInAscendingOrder(available)
	return sorted[index], nil
}

// DownloadFromBaseURL can download Bazel binaries from a specific URL while ignoring the predefined repositories.
func (r *Repositories) DownloadFromBaseURL(baseURL, version, destDir, destFile string) (string, error) {
	if !r.supportsBaseURL {
		return "", fmt.Errorf("downloads from %s are forbidden", BaseURLEnv)
	} else if baseURL == "" {
		return "", fmt.Errorf("%s is not set", BaseURLEnv)
	}

	srcFile, err := platforms.DetermineBazelFilename(version, true)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/%s/%s", baseURL, version, srcFile)
	return httputil.DownloadBinary(url, destDir, destFile)
}

// CreateRepositories creates a new Repositories instance with the given repositories. Any nil repository will be replaced by a dummy repository that raises an error whenever a download is attempted.
func CreateRepositories(releases ReleaseRepo, candidates CandidateRepo, fork ForkRepo, commits CommitRepo, rolling RollingRepo, supportsBaseURL bool) *Repositories {
	repos := &Repositories{supportsBaseURL: supportsBaseURL}

	if releases == nil {
		repos.Releases = &noReleaseRepo{err: errors.New("Bazel LTS releases are not supported")}
	} else {
		repos.Releases = releases
	}

	if candidates == nil {
		repos.Candidates = &noCandidateRepo{err: errors.New("Bazel release candidates are not supported")}
	} else {
		repos.Candidates = candidates
	}

	if fork == nil {
		repos.Fork = &noForkRepo{err: errors.New("forked versions of Bazel are not supported")}
	} else {
		repos.Fork = fork
	}

	if commits == nil {
		repos.Commits = &noCommitRepo{err: errors.New("Bazel versions built at commits are not supported")}
	} else {
		repos.Commits = commits
	}

	if rolling == nil {
		repos.Rolling = &noRollingRepo{err: errors.New("Bazel rolling releases are not supported")}
	} else {
		repos.Rolling = rolling
	}

	return repos
}

// The whole point of the structs below this line is that users can simply call repos.Releases.GetReleaseVersions()
// (etc) without having to worry whether `Releases` points at an actual repo.

type noReleaseRepo struct {
	err error
}

func (nrr *noReleaseRepo) GetReleaseVersions(bazeliskHome string, lastN int) ([]string, error) {
	return nil, nrr.err
}

func (nrr *noReleaseRepo) DownloadRelease(version, destDir, destFile string) (string, error) {
	return "", nrr.err
}

type noCandidateRepo struct {
	err error
}

func (ncc *noCandidateRepo) GetCandidateVersions(bazeliskHome string) ([]string, error) {
	return nil, ncc.err
}

func (ncc *noCandidateRepo) DownloadCandidate(version, destDir, destFile string) (string, error) {
	return "", ncc.err
}

type noForkRepo struct {
	err error
}

func (nfr *noForkRepo) GetVersions(bazeliskHome, fork string) ([]string, error) {
	return nil, nfr.err
}

func (nfr *noForkRepo) DownloadVersion(fork, version, destDir, destFile string) (string, error) {
	return "", nfr.err
}

type noCommitRepo struct {
	err error
}

func (nlgr *noCommitRepo) GetLastGreenCommit(bazeliskHome string, downstreamGreen bool) (string, error) {
	return "", nlgr.err
}

func (nlgr *noCommitRepo) DownloadAtCommit(commit, destDir, destFile string) (string, error) {
	return "", nlgr.err
}

type noRollingRepo struct {
	err error
}

func (nrr *noRollingRepo) GetRollingVersions(bazeliskHome string) ([]string, error) {
	return nil, nrr.err
}

func (nrr *noRollingRepo) DownloadRolling(version, destDir, destFile string) (string, error) {
	return "", nrr.err
}
