package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/platforms"
)

const (
	BaseURLEnv    = "BAZELISK_BASE_URL"
	BazelUpstream = "bazelbuild"
)

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

type LastGreenRepo interface {
	GetLastGreenVersion(bazeliskHome string, downstreamGreen bool) (string, error)
	DownloadLastGreen(commit, destDir, destFile string) (string, error)
}

type Repositories struct {
	Releases        ReleaseRepo
	Candidates      CandidateRepo
	Fork            ForkRepo
	LastGreen       LastGreenRepo
	supportsBaseURL bool
}

func IsFork(value string) bool {
	return value != "" && value != BazelUpstream
}

func (r *Repositories) DownloadFromRepo(fork, version string, isCommit bool, destDir, destFile string) (string, error) {
	switch {
	case IsFork(fork):
		return r.Fork.DownloadVersion(fork, version, destDir, destFile)
	case isCommit:
		return r.LastGreen.DownloadLastGreen(version, destDir, destFile)
	case strings.Contains(version, "rc"):
		return r.Candidates.DownloadCandidate(version, destDir, destFile)
	default:
		return r.Releases.DownloadRelease(version, destDir, destFile)
	}
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

func CreateRepositories(releases ReleaseRepo, candidates CandidateRepo, fork ForkRepo, lastGreen LastGreenRepo, supportsBaseURL bool) *Repositories {
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

	if lastGreen == nil {
		repos.LastGreen = &noLastGreenRepo{errors.New("Bazel-at-last-green versions are not supported")}
	} else {
		repos.LastGreen = lastGreen
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

type noLastGreenRepo struct {
	Error error
}

func (nlgr *noLastGreenRepo) GetLastGreenVersion(bazeliskHome string, downstreamGreen bool) (string, error) {
	return "", nlgr.Error
}

func (nlgr *noLastGreenRepo) DownloadLastGreen(commit, destDir, destFile string) (string, error) {
	return "", nlgr.Error
}
