package main

import (
	"fmt"
	"sort"
	"testing"

	"github.com/bazelbuild/bazelisk/core"
	"github.com/bazelbuild/bazelisk/runfiles"
)

func TestScanIssuesForIncompatibleFlags(t *testing.T) {
	samplesJSON, err := runfiles.ReadFile("sample-issues-migration.json")
	if err != nil {
		t.Errorf("Can not load sample github issues")
	}
	issues, err := core.ParseIssues(samplesJSON)
	if err != nil {
		t.Errorf("Can not parse sample github issues: %v", err)
	}

	flags := core.ScanIssuesForIncompatibleFlags(issues)
	expectedFlagnames := []string{
		"--//some/path:incompatible_user_defined_flag",
		"--incompatible_always_check_depset_elements",
		"--incompatible_no_implicit_file_export",
		"--incompatible_remove_enabled_toolchain_types",
		"--incompatible_remove_ram_utilization_factor",
		"--incompatible_validate_top_level_header_inclusions",
	}
	var gotFlags []string
	for _, flag := range flags {
		fmt.Printf("%s\n", flag.String())
		gotFlags = append(gotFlags, flag.Name)
	}
	sort.Strings(gotFlags)
	mismatch := false
	for i, got := range gotFlags {
		if expectedFlagnames[i] != got {
			mismatch = true
			break
		}
	}
	if mismatch || len(expectedFlagnames) != len(gotFlags) {
		t.Errorf("Expected %s, got %s", expectedFlagnames, gotFlags)
	}
}
