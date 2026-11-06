package core

import (
	"errors"
	"fmt"
	"strings"
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

func TestBuildURLFromFormatSelectsSHA256(t *testing.T) {
	const version = "9.0.0"
	osName, err := platforms.DetermineOperatingSystem()
	if err != nil {
		t.Fatal(err)
	}
	arch, err := platforms.DetermineArchitecture(osName, version)
	if err != nil {
		t.Fatal(err)
	}
	suffix := strings.ToUpper(osName + "_" + arch)
	platformKey := "BAZELISK_VERIFY_SHA256_" + suffix
	noJDKKey := "BAZELISK_VERIFY_SHA256_NOJDK_" + suffix
	for _, tc := range []struct {
		name   string
		values map[string]string
		want   string
	}{
		{name: "no hash", values: map[string]string{}},
		{name: "generic preserves case", values: map[string]string{"BAZELISK_VERIFY_SHA256": "GenericHash"}, want: "GenericHash"},
		{name: "platform only preserves case", values: map[string]string{platformKey: "PlatformHash"}, want: "PlatformHash"},
		{name: "platform before generic", values: map[string]string{platformKey: "PlatformHash", "BAZELISK_VERIFY_SHA256": "GenericHash"}, want: "PlatformHash"},
		{name: "NOJDK before platform", values: map[string]string{"BAZELISK_NOJDK": "1", noJDKKey: "NoJDKHash", platformKey: "PlatformHash"}, want: "NoJDKHash"},
		{name: "NOJDK falls back to platform", values: map[string]string{"BAZELISK_NOJDK": "1", platformKey: "PlatformHash"}, want: "PlatformHash"},
		{name: "NOJDK falls back to generic", values: map[string]string{"BAZELISK_NOJDK": "1", "BAZELISK_VERIFY_SHA256": "GenericHash"}, want: "GenericHash"},
		{name: "disabled NOJDK", values: map[string]string{"BAZELISK_NOJDK": "0", noJDKKey: "NoJDKHash", platformKey: "PlatformHash"}, want: "PlatformHash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildURLFromFormat(config.Static(tc.values), "https://example.test/%h/bazel", version)
			want := "https://example.test/" + tc.want + "/bazel"
			if err != nil || got != want {
				t.Errorf("URL = %q, error %v, want %q", got, err, want)
			}
		})
	}
}
