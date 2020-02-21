package main

import (
	"fmt"
	"io/ioutil"
	"sort"
	"testing"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
)

func TestScanIssuesForIncompatibleFlags(t *testing.T) {
	path, err := bazel.Runfile("sample-issues-migration.json")
	if err != nil {
		t.Errorf("Can not load sample github issues")
	}
	samplesJSON, err := ioutil.ReadFile(path)
	if err != nil {
		t.Errorf("Can not load sample github issues")
	}
	flags, err := scanIssuesForIncompatibleFlags(samplesJSON)
	if flags == nil {
		t.Errorf("Could not parse sample issues")
	}
	expected_flagnames := []string{
		"--incompatible_always_check_depset_elements",
		"--incompatible_no_implicit_file_export",
		"--incompatible_remove_enabled_toolchain_types",
		"--incompatible_remove_local_resources",
		"--incompatible_remove_ram_utilization_factor",
		"--incompatible_validate_top_level_header_inclusions",
	}
	var got_flags []string
	for _, flag := range flags {
		fmt.Printf("%s\n", flag.String())
		got_flags = append(got_flags, flag.Name)
	}
	sort.Strings(got_flags)
	mismatch := false
	for i, got := range got_flags {
		if expected_flagnames[i] != got {
			mismatch = true
			break
		}
	}
	if mismatch || len(expected_flagnames) != len(got_flags) {
		t.Errorf("Expected %s, got %s", expected_flagnames, got_flags)
	}
}
