package platforms

import (
	"fmt"
	"runtime"
)

var platforms = map[string]string{"darwin": "macos", "linux": "ubuntu1404", "windows": "windows"}

func GetPlatform() string {
	return platforms[runtime.GOOS]
}

func DetermineExecutableFilenameSuffix() string {
	filenameSuffix := ""
	if runtime.GOOS == "windows" {
		filenameSuffix = ".exe"
	}
	return filenameSuffix
}

func DetermineBazelFilename(version string, includeSuffix bool) (string, error) {
	var machineName string
	switch runtime.GOARCH {
	case "amd64":
		machineName = "x86_64"
	case "arm64":
		machineName = "arm64"
	default:
		return "", fmt.Errorf("unsupported machine architecture \"%s\", must be arm64 or x86_64", runtime.GOARCH)
	}

	var osName string
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		osName = runtime.GOOS
	default:
		return "", fmt.Errorf("unsupported operating system \"%s\", must be Linux, macOS or Windows", runtime.GOOS)
	}

	var filenameSuffix string
	if includeSuffix {
		filenameSuffix = DetermineExecutableFilenameSuffix()
	}

	return fmt.Sprintf("bazel-%s-%s-%s%s", version, osName, machineName, filenameSuffix), nil
}
