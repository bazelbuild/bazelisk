// Package versions contains functions to parse and sort Bazel version identifier.
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
	// BazelUpstream contains the name of the official Bazel GitHub organization.
	BazelUpstream = "bazelbuild"
)

var (
	releasePattern       = regexp.MustCompile(`^(\d+)\.\d+\.\d+$`)
	trackPattern         = regexp.MustCompile(`^(\d+)\.(x|\*)$`)
	patchPattern         = regexp.MustCompile(`^(\d+\.\d+\.\d+)-([\w\d]+)$`)
	candidatePattern     = regexp.MustCompile(`^(\d+\.\d+\.\d+)rc(\d+)$`)
	rollingPattern       = regexp.MustCompile(`^\d+\.0\.0-pre\.\d{8}(\.\d+){1,2}$`)
	latestReleasePattern = regexp.MustCompile(`^latest(?:-(?P<offset>\d+))?$`)
	commitPattern        = regexp.MustCompile(`^[a-z0-9]{40}$`)
)

// Info represents a structured Bazel version identifier.
type Info struct {
	MustBeRelease, MustBeCandidate bool
	IsLTS, IsRolling               bool
	IsCommit, IsFork, IsRelative   bool
	Fork, Value                    string
	LatestOffset, TrackRestriction int
}

// Parse extracts and returns structured information about the given Bazel version label.
func Parse(fork, version string) (*Info, error) {
	vi := &Info{Fork: fork, Value: version, IsFork: isFork(fork)}

	if m := releasePattern.FindStringSubmatch(version); m != nil {
		vi.IsLTS = true
		vi.MustBeRelease = true
	} else if m := trackPattern.FindStringSubmatch(version); m != nil {
		track, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("invalid version %q, expected something like '5.x' or '5.*'", version)
		}
		vi.IsLTS = true
		vi.MustBeRelease = (m[2] == "x")
		vi.IsRelative = true
		vi.TrackRestriction = track
	} else if patchPattern.MatchString(version) {
		vi.IsLTS = true
		vi.MustBeRelease = true
	} else if m := latestReleasePattern.FindStringSubmatch(version); m != nil {
		vi.IsLTS = true
		vi.MustBeRelease = true
		vi.IsRelative = true
		if m[1] != "" {
			offset, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, fmt.Errorf("invalid version %q, could not parse offset: %v", version, err)
			}
			vi.LatestOffset = offset
		}
	} else if candidatePattern.MatchString(version) {
		vi.IsLTS = true
		vi.MustBeCandidate = true
	} else if version == "last_rc" {
		vi.IsLTS = true
		vi.MustBeCandidate = true
		vi.IsRelative = true
	} else if commitPattern.MatchString(version) {
		vi.IsCommit = true
	} else if version == "last_green" {
		vi.IsCommit = true
		vi.IsRelative = true
	} else if rollingPattern.MatchString(version) {
		vi.IsRolling = true
	} else if version == "rolling" {
		vi.IsRolling = true
		vi.IsRelative = true
	} else {
		return nil, fmt.Errorf("invalid version '%s'", version)
	}
	return vi, nil
}

func isFork(value string) bool {
	return value != "" && value != BazelUpstream
}

// GetInAscendingOrder returns the given versions sorted in ascending order.
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

// MatchCommitPattern returns whether the given version refers to a commit,
// not including last_green.
func MatchCommitPattern(version string) bool {
	return commitPattern.MatchString(version)
}

// IsCommit returns whether the given version refers to a commit, including
// last_green.
func IsCommit(version string) bool {
	return version == "last_green" || commitPattern.MatchString(version)
}
