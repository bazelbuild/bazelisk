// Package platforms determines file names and extensions based on the current operating system.
package platforms

import (
	"fmt"
	"log"
	"runtime"

	"github.com/bazelbuild/bazelisk/config"
	"github.com/bazelbuild/bazelisk/versions"
	semver "github.com/hashicorp/go-version"
)

type platform struct {
	Name           string
	HasArm64Binary bool
}

var supportedPlatforms = map[string]*platform{
	"darwin": {
		Name:           "macos",
		HasArm64Binary: true,
	},
	"linux": {
		Name:           "linux",
		HasArm64Binary: true,
	},
	"windows": {
		Name:           "windows",
		HasArm64Binary: true,
	},
}

// GetPlatform returns a Bazel CI-compatible platform identifier for the current operating system.
// TODO(fweikert): raise an error for unsupported platforms
func GetPlatform() (string, error) {
	platform := supportedPlatforms[runtime.GOOS]
	arch := runtime.GOARCH
	if arch == "arm64" {
		if platform.HasArm64Binary {
			return platform.Name + "_arm64", nil
		}
		return "", fmt.Errorf("arm64 %s is unsupported", runtime.GOOS)
	}

	return platform.Name, nil
}

// DetermineExecutableFilenameSuffix returns the extension for binaries on the current operating system.
func DetermineExecutableFilenameSuffix() string {
	filenameSuffix := ""
	if runtime.GOOS == "windows" {
		filenameSuffix = ".exe"
	}
	return filenameSuffix
}

// DetermineArchitecture returns the architecture of the current machine.
func DetermineArchitecture(osName, version string) (string, error) {
	var machineName string
	switch runtime.GOARCH {
	case "amd64":
		machineName = "x86_64"
	case "arm64":
		machineName = "arm64"
	default:
		return "", fmt.Errorf("unsupported machine architecture %q, must be arm64 or x86_64", runtime.GOARCH)
	}

	if osName == "darwin" {
		machineName = DarwinFallback(machineName, version)
	}

	return machineName, nil
}

// DetermineOperatingSystem returns the name of the operating system.
func DetermineOperatingSystem() (string, error) {
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		return runtime.GOOS, nil
	default:
		return "", fmt.Errorf("unsupported operating system %q, must be Linux, macOS or Windows", runtime.GOOS)
	}
}

// DetermineBazelFilename returns the correct file name of a local Bazel binary.
func DetermineBazelFilename(version string, includeSuffix bool, config config.Config) (string, error) {
	flavor := "bazel"

	bazeliskNojdk := config.Get("BAZELISK_NOJDK")

	if len(bazeliskNojdk) != 0 && bazeliskNojdk != "0" {
		flavor = "bazel_nojdk"
	}

	osName, err := DetermineOperatingSystem()
	if err != nil {
		return "", err
	}

	machineName, err := DetermineArchitecture(osName, version)
	if err != nil {
		return "", err
	}

	var filenameSuffix string
	if includeSuffix {
		filenameSuffix = DetermineExecutableFilenameSuffix()
	}

	return fmt.Sprintf("%s-%s-%s-%s%s", flavor, version, osName, machineName, filenameSuffix), nil
}

// DetermineBazelInstallerFilename returns the correct file name of a Bazel installer script.
func DetermineBazelInstallerFilename(version string, config config.Config) (string, error) {
	osName, err := DetermineOperatingSystem()
	if err != nil {
		return "", err
	}

	machineName, err := DetermineArchitecture(osName, version)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("bazel-%s-installer-%s-%s.sh", version, osName, machineName), nil
}

// DarwinFallback Darwin arm64 was supported since 4.1.0, before 4.1.0, fall back to x86_64
func DarwinFallback(machineName string, version string) (alterMachineName string) {
	// Do not use fallback for commits since they are likely newer than Bazel 4.1
	if versions.IsCommit(version) {
		return machineName
	}

	v, err := semver.NewVersion(version)
	if err != nil {
		return machineName
	}

	armSupportVer, _ := semver.NewVersion("4.1.0")

	if machineName == "arm64" && v.LessThan(armSupportVer) {
		log.Printf("WARN: Fallback to x86_64 because arm64 is not supported on Apple Silicon until 4.1.0")
		return "x86_64"
	}
	return machineName
}
