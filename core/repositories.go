package core

import (
	"errors"
	"fmt"

	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/platforms"
	"github.com/bazelbuild/bazelisk/versions"
)

const (
	BaseURLEnv = "BAZELISK_BASE_URL"
)

type DownloadFunc func(destDir, destFile string) (string, error)

type ReleaseRepo interface {
	GetReleaseVersions(bazeliskHome string) ([]string, error)
	DownloadRelease(version, destDir, destFile string) (string, error)
}

type CandidateRepo interface {
	GetCandidateVersions(bazeliskHome string) ([]string, error)
	DownloadCandidate(version, destDir, destFile string) (string, error)
}

type ForkRepo interface {
	GetVersions(bazeliskHome, fork string) ([]string, error)
	DownloadVersion(fork, version, destDir, destFile string) (string, error)
}

type CommitRepo interface {
	GetLastGreenCommit(bazeliskHome string, downstreamGreen bool) (string, error)
	DownloadAtCommit(commit, destDir, destFile string) (string, error)
}

type Repositories struct {
	Releases        ReleaseRepo
	Candidates      CandidateRepo
	Fork            ForkRepo
	Commits         CommitRepo
	supportsBaseURL bool
}

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
	}

	return "", nil, fmt.Errorf("Unsupported version identifier '%s'", version)
}

func (r *Repositories) resolveFork(bazeliskHome string, vi *versions.VersionInfo) (string, DownloadFunc, error) {
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

func (r *Repositories) resolveRelease(bazeliskHome string, vi *versions.VersionInfo) (string, DownloadFunc, error) {
	version, err := resolvePotentiallyRelativeVersion(bazeliskHome, r.Releases.GetReleaseVersions, vi)
	if err != nil {
		return "", nil, err
	}
	downloader := func(destDir, destFile string) (string, error) {
		return r.Releases.DownloadRelease(version, destDir, destFile)
	}
	return version, downloader, nil
}

func (r *Repositories) resolveCandidate(bazeliskHome string, vi *versions.VersionInfo) (string, DownloadFunc, error) {
	version, err := resolvePotentiallyRelativeVersion(bazeliskHome, r.Candidates.GetCandidateVersions, vi)
	if err != nil {
		return "", nil, err
	}
	downloader := func(destDir, destFile string) (string, error) {
		return r.Candidates.DownloadCandidate(version, destDir, destFile)
	}
	return version, downloader, nil
}

func (r *Repositories) resolveCommit(bazeliskHome string, vi *versions.VersionInfo) (string, DownloadFunc, error) {
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

type listVersionsFunc func(bazeliskHome string) ([]string, error)

func resolvePotentiallyRelativeVersion(bazeliskHome string, lister listVersionsFunc, vi *versions.VersionInfo) (string, error) {
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

func CreateRepositories(releases ReleaseRepo, candidates CandidateRepo, fork ForkRepo, commits CommitRepo, supportsBaseURL bool) *Repositories {
	repos := &Repositories{supportsBaseURL: supportsBaseURL}

	if releases == nil {
		repos.Releases = &noReleaseRepo{errors.New("official Bazel releases are not supported")}
	} else {
		repos.Releases = releases
	}

	if candidates == nil {
		repos.Candidates = &noCandidateRepo{errors.New("Bazel release candidates are not supported")}
	} else {
		repos.Candidates = candidates
	}

	if fork == nil {
		repos.Fork = &noForkRepo{errors.New("forked versions of Bazel are not supported")}
	} else {
		repos.Fork = fork
	}

	if commits == nil {
		repos.Commits = &noCommitRepo{errors.New("Bazel versions built at commits are not supported")}
	} else {
		repos.Commits = commits
	}

	return repos
}

// The whole point of the structs below this line is that users can simply call repos.Releases.GetReleaseVersions()
// (etc) without having to worry whether `Releases` points at an actual repo.
type noReleaseRepo struct {
	Error error
}

func (nrr *noReleaseRepo) GetReleaseVersions(bazeliskHome string) ([]string, error) {
	return []string{}, nrr.Error
}

func (nrr *noReleaseRepo) DownloadRelease(version, destDir, destFile string) (string, error) {
	return "", nrr.Error
}

type noCandidateRepo struct {
	Error error
}

func (ncc *noCandidateRepo) GetCandidateVersions(bazeliskHome string) ([]string, error) {
	return []string{}, ncc.Error
}

func (ncc *noCandidateRepo) DownloadCandidate(version, destDir, destFile string) (string, error) {
	return "", ncc.Error
}

type noForkRepo struct {
	Error error
}

func (nfr *noForkRepo) GetVersions(bazeliskHome, fork string) ([]string, error) {
	return []string{}, nfr.Error
}

func (nfr *noForkRepo) DownloadVersion(fork, version, destDir, destFile string) (string, error) {
	return "", nfr.Error
}

type noCommitRepo struct {
	Error error
}

func (nlgr *noCommitRepo) GetLastGreenCommit(bazeliskHome string, downstreamGreen bool) (string, error) {
	return "", nlgr.Error
}

func (nlgr *noCommitRepo) DownloadAtCommit(commit, destDir, destFile string) (string, error) {
	return "", nlgr.Error
}
