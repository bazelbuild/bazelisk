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
	releasePattern       = regexp.MustCompile(`^(\d+\.\d+\.\d+)$`)
	candidatePattern     = regexp.MustCompile(`^(\d+\.\d+\.\d+)rc(\d+)$`)
	rollingPattern       = regexp.MustCompile(`^\d+\.0\.0-pre\.\d{8}(\.\d+){1,2}$`)
	latestReleasePattern = regexp.MustCompile(`^latest(?:-(?P<offset>\d+))?$`)
	commitPattern        = regexp.MustCompile(`^[a-z0-9]{40}$`)
)

// Info represents a structured Bazel version identifier.
type Info struct {
	IsRelease, IsCandidate, IsCommit, IsFork, IsRolling, IsRelative, IsDownstream bool
	Fork, Value                                                        string
	LatestOffset                                                       int
}

// Parse extracts and returns structured information about the given Bazel version label.
func Parse(fork, version string) (*Info, error) {
	vi := &Info{Fork: fork, Value: version, IsFork: isFork(fork)}

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
	} else if rollingPattern.MatchString(version) {
		vi.IsRolling = true
	} else if version == "rolling" {
		vi.IsRolling = true
		vi.IsRelative = true
	} else {
		return nil, fmt.Errorf("Invalid version '%s'", version)
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
