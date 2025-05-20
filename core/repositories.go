package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bazelbuild/bazelisk/config"
	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/platforms"
	"github.com/bazelbuild/bazelisk/versions"
)

const (
	// BaseURLEnv is the name of the environment variable that stores the base URL for downloads.
	BaseURLEnv = "BAZELISK_BASE_URL"

	// FormatURLEnv is the name of the environment variable that stores the format string to generate URLs for downloads.
	FormatURLEnv = "BAZELISK_FORMAT_URL"
)

// DownloadFunc downloads a specific Bazel binary to the given location and returns the absolute path.
type DownloadFunc func(destDir, destFile string) (string, error)

// LTSFilter filters Bazel versions based on specific criteria.
type LTSFilter func(string) bool

// FilterOpts represents options relevant to filtering Bazel versions.
type FilterOpts struct {
	MaxResults int
	Track      int
	Filter     LTSFilter
}

// LTSRepo represents a repository that stores LTS Bazel releases and their candidates.
type LTSRepo interface {
	// GetLTSVersions returns a list of all available LTS release (candidates) that match the given filter options.
	// Warning: Filters only work reliably if the versions are processed in descending order!
	GetLTSVersions(bazeliskHome string, opts *FilterOpts) ([]string, error)

	// DownloadLTS downloads the given Bazel version into the specified location and returns the absolute path.
	DownloadLTS(version, destDir, destFile string, config config.Config) (string, error)
}

// ForkRepo represents a repository that stores a fork of Bazel (releases).
type ForkRepo interface {
	// GetVersions returns the versions of all available Bazel binaries in the given fork.
	GetVersions(bazeliskHome, fork string) ([]string, error)

	// DownloadVersion downloads the given Bazel binary from the specified fork into the given location and returns the absolute path.
	DownloadVersion(fork, version, destDir, destFile string, config config.Config) (string, error)
}

// CommitRepo represents a repository that stores Bazel binaries built at specific commits.
// It can also return the hashes of the most recent commits that passed Bazel CI pipelines successfully.
type CommitRepo interface {
	// GetLastGreenCommit returns the most recent commit at which a Bazel binary is successfully built.
	GetLastGreenCommit(bazeliskHome string) (string, error)

	// DownloadAtCommit downloads a Bazel binary built at the given commit into the specified location and returns the absolute path.
	DownloadAtCommit(commit, destDir, destFile string, config config.Config) (string, error)
}

// RollingRepo represents a repository that stores rolling Bazel releases.
type RollingRepo interface {
	// GetRollingVersions returns a list of all available rolling release versions.
	GetRollingVersions(bazeliskHome string) ([]string, error)

	// DownloadRolling downloads the given Bazel version into the specified location and returns the absolute path.
	DownloadRolling(version, destDir, destFile string, config config.Config) (string, error)
}

// Repositories offers access to different types of Bazel repositories, mainly for finding and downloading the correct version of Bazel.
type Repositories struct {
	LTS             LTSRepo
	Fork            ForkRepo
	Commits         CommitRepo
	Rolling         RollingRepo
	supportsBaseOrFormatURL bool
}

// ResolveVersion resolves a potentially relative Bazel version string such as "latest" to an absolute version identifier, and returns this identifier alongside a function to download said version.
func (r *Repositories) ResolveVersion(bazeliskHome, fork, version string, config config.Config) (string, DownloadFunc, error) {
	vi, err := versions.Parse(fork, version)
	if err != nil {
		return "", nil, err
	}

	if vi.IsFork {
		return r.resolveFork(bazeliskHome, vi, config)
	} else if vi.IsLTS {
		return r.resolveLTS(bazeliskHome, vi, config)
	} else if vi.IsCommit {
		return r.resolveCommit(bazeliskHome, vi, config)
	} else if vi.IsRolling {
		return r.resolveRolling(bazeliskHome, vi, config)
	}

	return "", nil, fmt.Errorf("unsupported version identifier '%s'", version)
}

func (r *Repositories) resolveFork(bazeliskHome string, vi *versions.Info, config config.Config) (string, DownloadFunc, error) {
	if vi.IsRelative && (vi.MustBeCandidate || vi.IsCommit) {
		return "", nil, errors.New("forks do not support last_rc and last_green")
	}
	lister := func(bazeliskHome string) ([]string, error) {
		return r.Fork.GetVersions(bazeliskHome, vi.Fork)
	}
	version, err := resolvePotentiallyRelativeVersion(bazeliskHome, lister, vi)
	if err != nil {
		return "", nil, err
	}
	downloader := func(destDir, destFile string) (string, error) {
		return r.Fork.DownloadVersion(vi.Fork, version, destDir, destFile, config)
	}
	return version, downloader, nil
}

// IsRelease returns whether a version string is for a final release.
var IsRelease = func(version string) bool {
	return !strings.Contains(version, "rc")
}

// IsCandidate returns whether a version string is for a release candidate.
var IsCandidate = func(version string) bool {
	return strings.Contains(version, "rc")
}

func (r *Repositories) resolveLTS(bazeliskHome string, vi *versions.Info, config config.Config) (string, DownloadFunc, error) {
	opts := &FilterOpts{
		// Optimization: only fetch last (x+1) releases if the version is "latest-x".
		MaxResults: vi.LatestOffset + 1,
		Track:      vi.TrackRestriction,
	}

	if vi.MustBeRelease {
		opts.Filter = IsRelease
	} else if vi.MustBeCandidate {
		opts.Filter = IsCandidate
	} else {
		// Wildcard -> can be either release or candidate
		opts.Filter = func(v string) bool { return true }
	}

	lister := func(bazeliskHome string) ([]string, error) {
		return r.LTS.GetLTSVersions(bazeliskHome, opts)
	}
	version, err := resolvePotentiallyRelativeVersion(bazeliskHome, lister, vi)
	if err != nil {
		return "", nil, err
	}
	downloader := func(destDir, destFile string) (string, error) {
		return r.LTS.DownloadLTS(version, destDir, destFile, config)
	}
	return version, downloader, nil
}

func (r *Repositories) resolveCommit(bazeliskHome string, vi *versions.Info, config config.Config) (string, DownloadFunc, error) {
	version := vi.Value
	if vi.IsRelative {
		var err error
		version, err = r.Commits.GetLastGreenCommit(bazeliskHome)
		if err != nil {
			return "", nil, fmt.Errorf("cannot resolve last green commit: %v", err)
		}
	}
	downloader := func(destDir, destFile string) (string, error) {
		return r.Commits.DownloadAtCommit(version, destDir, destFile, config)
	}
	return version, downloader, nil
}

func (r *Repositories) resolveRolling(bazeliskHome string, vi *versions.Info, config config.Config) (string, DownloadFunc, error) {
	lister := func(bazeliskHome string) ([]string, error) {
		return r.Rolling.GetRollingVersions(bazeliskHome)
	}
	version, err := resolvePotentiallyRelativeVersion(bazeliskHome, lister, vi)
	if err != nil {
		return "", nil, err
	}
	downloader := func(destDir, destFile string) (string, error) {
		return r.Rolling.DownloadRolling(version, destDir, destFile, config)
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
		return "", fmt.Errorf("cannot resolve version %q: There are not enough matching Bazel releases (%d)", vi.Value, len(available))
	}
	sorted := versions.GetInAscendingOrder(available)
	return sorted[index], nil
}

// DownloadFromBaseURL can download Bazel binaries from a specific URL while ignoring the predefined repositories.
func (r *Repositories) DownloadFromBaseURL(baseURL, version, destDir, destFile string, config config.Config) (string, error) {
	if !r.supportsBaseOrFormatURL {
		return "", fmt.Errorf("downloads from %s are forbidden", BaseURLEnv)
	}
	if baseURL == "" {
		return "", fmt.Errorf("%s is not set", BaseURLEnv)
	}

	srcFile, err := platforms.DetermineBazelFilename(version, true, config)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/%s/%s", baseURL, version, srcFile)
	return httputil.DownloadBinary(url, destDir, destFile, config)
}

// BuildURLFromFormat returns a Bazel download URL based on formatURL.
func BuildURLFromFormat(config config.Config, formatURL, version string) (string, error) {
	osName, err := platforms.DetermineOperatingSystem()
	if err != nil {
		return "", err
	}

	machineName, err := platforms.DetermineArchitecture(osName, version)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.Grow(len(formatURL) * 2) // Approximation.
	for i := 0; i < len(formatURL); i++ {
		ch := formatURL[i]
		if ch == '%' {
			i++
			if i == len(formatURL) {
				return "", errors.New("trailing %")
			}

			ch = formatURL[i]
			switch ch {
			case 'e':
				b.WriteString(platforms.DetermineExecutableFilenameSuffix())
			case 'h':
				b.WriteString(config.Get("BAZELISK_VERIFY_SHA256"))
			case 'm':
				b.WriteString(machineName)
			case 'o':
				b.WriteString(osName)
			case 'v':
				b.WriteString(version)
			case '%':
				b.WriteByte('%')
			default:
				return "", fmt.Errorf("unknown placeholder %%%c", ch)
			}
		} else {
			b.WriteByte(ch)
		}
	}
	return b.String(), nil
}

// DownloadFromFormatURL can download Bazel binaries from a specific URL while ignoring the predefined repositories.
func (r *Repositories) DownloadFromFormatURL(config config.Config, formatURL, version, destDir, destFile string) (string, error) {
	if !r.supportsBaseOrFormatURL {
		return "", fmt.Errorf("downloads from %s are forbidden", FormatURLEnv)
	}
	if formatURL == "" {
		return "", fmt.Errorf("%s is not set", FormatURLEnv)
	}

	url, err := BuildURLFromFormat(config, formatURL, version)
	if err != nil {
		return "", err
	}

	return httputil.DownloadBinary(url, destDir, destFile, config)
}

// CreateRepositories creates a new Repositories instance with the given repositories. Any nil repository will be replaced by a dummy repository that raises an error whenever a download is attempted.
func CreateRepositories(lts LTSRepo, fork ForkRepo, commits CommitRepo, rolling RollingRepo, supportsBaseOrFormatURL bool) *Repositories {
	repos := &Repositories{supportsBaseOrFormatURL: supportsBaseOrFormatURL}

	if lts == nil {
		repos.LTS = &noLTSRepo{err: errors.New("Bazel LTS releases & candidates are not supported")}
	} else {
		repos.LTS = lts
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

// The whole point of the structs below this line is that users can simply call repos.LTS.GetLTSVersions()
// (etc) without having to worry whether `LTS` points at an actual repo.

type noLTSRepo struct {
	err error
}

func (nolts *noLTSRepo) GetLTSVersions(bazeliskHome string, opts *FilterOpts) ([]string, error) {
	return nil, nolts.err
}

func (nolts *noLTSRepo) DownloadLTS(version, destDir, destFile string, config config.Config) (string, error) {
	return "", nolts.err
}

type noForkRepo struct {
	err error
}

func (nfr *noForkRepo) GetVersions(bazeliskHome, fork string) ([]string, error) {
	return nil, nfr.err
}

func (nfr *noForkRepo) DownloadVersion(fork, version, destDir, destFile string, config config.Config) (string, error) {
	return "", nfr.err
}

type noCommitRepo struct {
	err error
}

func (nlgr *noCommitRepo) GetLastGreenCommit(bazeliskHome string) (string, error) {
	return "", nlgr.err
}

func (nlgr *noCommitRepo) DownloadAtCommit(commit, destDir, destFile string, config config.Config) (string, error) {
	return "", nlgr.err
}

type noRollingRepo struct {
	err error
}

func (nrr *noRollingRepo) GetRollingVersions(bazeliskHome string) ([]string, error) {
	return nil, nrr.err
}

func (nrr *noRollingRepo) DownloadRolling(version, destDir, destFile string, config config.Config) (string, error) {
	return "", nrr.err
}
