package versions

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"

	"github.com/hashicorp/go-version"
)

const (
	BazelUpstream = "bazelbuild"
)

var (
	releasePattern       = regexp.MustCompile(`^(\d+.\d+.\d+)$`)
	candidatePattern     = regexp.MustCompile(`^(\d+.\d+.\d+)rc(\d+)$`)
	latestReleasePattern = regexp.MustCompile(`^latest(?:-(?P<offset>\d+))?$`)
	commitPattern        = regexp.MustCompile(`^[a-z0-9]{40}$`)
)

type VersionInfo struct {
	IsRelease, IsCandidate, IsCommit, IsFork, IsRelative, IsDownstream bool
	Fork, Value                                                        string
	LatestOffset                                                       int
}

func Parse(fork, version string) (*VersionInfo, error) {
	vi := &VersionInfo{Fork: fork, Value: version, IsFork: IsFork(fork)}

	if releasePattern.MatchString(version) {
		vi.IsRelease = true
	} else if m := latestReleasePattern.FindStringSubmatch(version); m != nil {
		vi.IsRelease = true
		vi.IsRelative = true
		if m[1] != "" {
			offset, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, fmt.Errorf("invalid version \"%s\", could not parse offset: %v", version, err)
			}
			vi.LatestOffset = offset
		}
	} else if candidatePattern.MatchString(version) {
		vi.IsCandidate = true
	} else if version == "last_rc" {
		vi.IsCandidate = true
		vi.IsRelative = true
	} else if commitPattern.MatchString(version) {
		vi.IsCommit = true
	} else if version == "last_green" {
		vi.IsCommit = true
		vi.IsRelative = true
	} else if version == "last_downstream_green" {
		vi.IsCommit = true
		vi.IsRelative = true
		vi.IsDownstream = true
	} else {
		return nil, fmt.Errorf("Invalid version '%s'", version)
	}
	return vi, nil
}

func IsFork(value string) bool {
	return value != "" && value != BazelUpstream
}

func GetInAscendingOrder(versions []string) []string {
	wrappers := make([]*version.Version, len(versions))
	for i, v := range versions {
		wrapper, err := version.NewVersion(v)
		if err != nil {
			log.Printf("WARN: Could not parse version: %s", v)
		}
		wrappers[i] = wrapper
	}
	sort.Sort(version.Collection(wrappers))

	sorted := make([]string, len(versions))
	for i, w := range wrappers {
		sorted[i] = w.Original()
	}
	return sorted
}
