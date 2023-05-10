package core

import (
	"errors"
	"fmt"
	"testing"

	"github.com/bazelbuild/bazelisk/config"
	"github.com/bazelbuild/bazelisk/platforms"
)

func TestBuildURLFromFormat(t *testing.T) {
	osName, err := platforms.DetermineOperatingSystem()
	if err != nil {
		t.Fatalf("Cannot get operating system name: %v", err)
	}

	version := "6.0.0"

	machineName, err := platforms.DetermineArchitecture(osName, version)
	if err != nil {
		t.Fatalf("Cannot get machine architecture name: %v", err)
	}

	suffix := platforms.DetermineExecutableFilenameSuffix()

	sha256 := "SomeSha256ValueThatIsIrrelevant"
	config := config.Static(map[string]string{
		"BAZELISK_VERIFY_SHA256": sha256,
	})

	type test struct {
		format  string
		want    string
		wantErr error
	}

	tests := []test{
		{format: "", want: ""},
		{format: "no/placeholders", want: "no/placeholders"},

		{format: "%", wantErr: errors.New("trailing %")},
		{format: "%%", want: "%"},
		{format: "%%%%", want: "%%"},
		{format: "invalid/trailing/%", wantErr: errors.New("trailing %")},
		{format: "escaped%%placeholder", want: "escaped%placeholder"},

		{format: "foo-%e-bar", want: fmt.Sprintf("foo-%s-bar", suffix)},
		{format: "foo-%h-bar", want: fmt.Sprintf("foo-%s-bar", sha256)},
		{format: "foo-%m-bar", want: fmt.Sprintf("foo-%s-bar", machineName)},
		{format: "foo-%o-bar", want: fmt.Sprintf("foo-%s-bar", osName)},
		{format: "foo-%v-bar", want: fmt.Sprintf("foo-%s-bar", version)},

		{format: "repeated %v %m %v", want: fmt.Sprintf("repeated %s %s %s", version, machineName, version)},

		{format: "https://real.example.com/%e/%m/%o/%v#%%20trailing", want: fmt.Sprintf("https://real.example.com/%s/%s/%s/%s#%%20trailing", suffix, machineName, osName, version)},
	}

	for _, tc := range tests {
		got, err := BuildURLFromFormat(config, tc.format, version)
		if fmt.Sprintf("%v", err) != fmt.Sprintf("%v", tc.wantErr) {
			if got != "" {
				t.Errorf("format '%s': got non-empty '%s' on error", tc.format, got)
			}
			t.Errorf("format '%s': got error %v, want error %v", tc.format, err, tc.wantErr)
		} else if got != tc.want {
			t.Errorf("format '%s': got %s, want %s", tc.format, got, tc.want)
		}
	}
}
