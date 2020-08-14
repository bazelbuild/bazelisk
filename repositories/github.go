package repositories

import (
	"encoding/json"
	"fmt"

	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/platforms"
)

const (
	urlPattern = "https://github.com/%s/bazel/releases/download/%s/%s"
)

type GitHubRepo struct {
	token string
}

func CreateGitHubRepo(token string) *GitHubRepo {
	return &GitHubRepo{token}
}

// ForkRepo
func (gh *GitHubRepo) GetVersions(bazeliskHome, bazelFork string) ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/bazel/releases", bazelFork)
	releasesJSON, err := httputil.MaybeDownload(bazeliskHome, url, bazelFork+"-releases.json", "list of Bazel releases from github.com/"+bazelFork, gh.token)
	if err != nil {
		return []string{}, fmt.Errorf("unable to dermine '%s' releases: %v", bazelFork, err)
	}

	var releases []gitHubRelease
	if err := json.Unmarshal(releasesJSON, &releases); err != nil {
		return []string{}, fmt.Errorf("could not parse JSON into list of releases: %v", err)
	}

	var tags []string
	for _, release := range releases {
		if release.Prerelease {
			continue
		}
		tags = append(tags, release.TagName)
	}
	return tags, nil
}

type gitHubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
}

func (gh *GitHubRepo) DownloadVersion(fork, version, destDir, destFile string) (string, error) {
	filename, err := platforms.DetermineBazelFilename(version, true)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf(urlPattern, fork, version, filename)
	return httputil.DownloadBinary(url, destDir, destFile)
}
