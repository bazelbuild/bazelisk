// Package core contains the core Bazelisk logic, as well as abstractions for Bazel repositories.
package core

// TODO: split this file into multiple smaller ones in dedicated packages (e.g. execution, incompatible, ...).

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/bazelbuild/bazelisk/config"
	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/platforms"
	"github.com/bazelbuild/bazelisk/versions"
	"github.com/bazelbuild/bazelisk/ws"
	"github.com/gofrs/flock"
	"github.com/mitchellh/go-homedir"
)

const (
	bazelReal               = "BAZEL_REAL"
	skipWrapperEnv          = "BAZELISK_SKIP_WRAPPER"
	bazeliskEnv             = "BAZELISK"
	defaultWrapperDirectory = "./tools"
	defaultWrapperName      = "bazel"
	maxDirLength            = 255
)

var (
	// BazeliskVersion is filled in via x_defs when building a release.
	BazeliskVersion = "development"
)

// ArgsFunc is a function that receives a resolved Bazel version and returns the arguments to invoke
// Bazel with.
type ArgsFunc func(resolvedBazelVersion string) []string

// MakeDefaultConfig returns a config based on env and .bazeliskrc files.
func MakeDefaultConfig() config.Config {
	configs := []config.Config{config.FromEnv()}

	workspaceConfigPath, err := config.LocateWorkspaceConfigFile()
	if err == nil {
		c, err := config.FromFile(workspaceConfigPath)
		if err != nil {
			log.Fatal(err)
		}
		configs = append(configs, c)
	}

	userConfigPath, err := config.LocateUserConfigFile()
	if err == nil {
		c, err := config.FromFile(userConfigPath)
		if err != nil {
			log.Fatal(err)
		}
		configs = append(configs, c)
	}
	return config.Layered(configs...)
}

// RunBazelisk runs the main Bazelisk logic for the given arguments and Bazel repositories.
func RunBazelisk(args []string, repos *Repositories) (int, error) {
	return RunBazeliskWithArgsFunc(func(_ string) []string { return args }, repos)
}

// RunBazeliskWithArgsFunc runs the main Bazelisk logic for the given ArgsFunc and Bazel
// repositories.
func RunBazeliskWithArgsFunc(argsFunc ArgsFunc, repos *Repositories) (int, error) {

	return RunBazeliskWithArgsFuncAndConfig(argsFunc, repos, MakeDefaultConfig())
}

// RunBazeliskWithArgsFuncAndConfig runs the main Bazelisk logic for the given ArgsFunc and Bazel
// repositories and config.
func RunBazeliskWithArgsFuncAndConfig(argsFunc ArgsFunc, repos *Repositories, config config.Config) (int, error) {
	return RunBazeliskWithArgsFuncAndConfigAndOut(argsFunc, repos, config, nil)
}

// RunBazeliskWithArgsFuncAndConfigAndOut runs the main Bazelisk logic for the given ArgsFunc and Bazel
// repositories and config, writing its stdout to the passed writer.
func RunBazeliskWithArgsFuncAndConfigAndOut(argsFunc ArgsFunc, repos *Repositories, config config.Config, out io.Writer) (int, error) {
	httputil.UserAgent = getUserAgent(config)

	bazelInstallation, err := GetBazelInstallation(repos, config)
	if err != nil {
		return -1, err
	}

	args := argsFunc(bazelInstallation.Version)

	// --print_env must be the first argument.
	if len(args) > 0 && args[0] == "--print_env" {
		// print environment variables for sub-processes
		cmd := makeBazelCmd(bazelInstallation.Path, args, nil, config)
		for _, val := range cmd.Env {
			fmt.Println(val)
		}
		return 0, nil
	}

	// --strict and --migrate and --bisect must be the first argument.
	if len(args) > 0 && (args[0] == "--strict" || args[0] == "--migrate") {
		cmd, err := getBazelCommand(args)
		if err != nil {
			return -1, err
		}
		newFlags, err := getIncompatibleFlags(bazelInstallation.Path, cmd, config)
		if err != nil {
			return -1, fmt.Errorf("could not get the list of incompatible flags: %v", err)
		}
		if args[0] == "--migrate" {
			migrate(bazelInstallation.Path, args[1:], newFlags, config)
		} else {
			// When --strict is present, it expands to the list of --incompatible_ flags
			// that should be enabled for the given Bazel version.
			args = insertArgs(args[1:], newFlags)
		}
	} else if len(args) > 0 && strings.HasPrefix(args[0], "--bisect") {
		// When --bisect is present, we run the bisect logic.
		if !strings.HasPrefix(args[0], "--bisect=") {
			return -1, fmt.Errorf("Error: --bisect must have a value. Expected format: '--bisect=[~]<good bazel commit>..<bad bazel commit>'")
		}
		value := args[0][len("--bisect="):]
		commits := strings.Split(value, "..")
		if len(commits) == 2 {
			bazeliskHome, err := getBazeliskHome(config)
			if err != nil {
				return -1, fmt.Errorf("could not determine Bazelisk home directory: %v", err)
			}

			bisect(commits[0], commits[1], args[1:], bazeliskHome, repos, config)
		} else {
			return -1, fmt.Errorf("Error: Invalid format for --bisect. Expected format: '--bisect=[~]<good bazel commit>..<bad bazel commit>'")
		}
	}

	// print bazelisk version information if "version" is the first non-flag argument
	// bazel version is executed after this command
	if ok, gnuFormat := isVersionCommand(args); ok {
		if gnuFormat {
			fmt.Printf("Bazelisk %s\n", BazeliskVersion)
		} else {
			fmt.Printf("Bazelisk version: %s\n", BazeliskVersion)
		}
	}

	// handle completion command
	if isCompletionCommand(args) {
		err := handleCompletionCommand(args, bazelInstallation, config)
		if err != nil {
			return -1, fmt.Errorf("could not handle completion command: %v", err)
		}
		return 0, nil
	}

	exitCode, err := runBazel(bazelInstallation.Path, args, out, config)
	if err != nil {
		return -1, fmt.Errorf("could not run Bazel: %v", err)
	}
	return exitCode, nil
}

func isVersionCommand(args []string) (result bool, gnuFormat bool) {
	for _, arg := range args {
		// Check if the --gnu_format flag is set, if that is the case,
		// the version is printed differently
		if arg == "--gnu_format" {
			gnuFormat = true
		} else if arg == "version" {
			result = true
		} else if !strings.HasPrefix(arg, "--") {
			return // First non-flag arg is not "version" -> it must be a different command
		}
		if result && gnuFormat {
			break
		}
	}
	return
}

// BazelInstallation provides a summary of a single install of `bazel`
type BazelInstallation struct {
	Version string
	Path    string
}

// GetBazelInstallation provides a mechanism to find the `bazel` binary to execute, as well as its version
func GetBazelInstallation(repos *Repositories, config config.Config) (*BazelInstallation, error) {
	bazeliskHome, err := getBazeliskHome(config)
	if err != nil {
		return nil, fmt.Errorf("could not determine Bazelisk home directory: %v", err)
	}

	err = os.MkdirAll(bazeliskHome, 0755)
	if err != nil {
		return nil, fmt.Errorf("could not create directory %s: %v", bazeliskHome, err)
	}

	bazelVersionString, err := GetBazelVersion(config)
	if err != nil {
		return nil, fmt.Errorf("could not get Bazel version: %v", err)
	}

	bazelPath, err := homedir.Expand(bazelVersionString)
	if err != nil {
		return nil, fmt.Errorf("could not expand home directory in path: %v", err)
	}

	var resolvedVersion string

	// If we aren't using a local Bazel binary, we'll have to parse the version string and
	// download the version that the user wants.
	if !filepath.IsAbs(bazelPath) {
		resolvedVersion = bazelVersionString
		bazelPath, err = downloadBazel(bazelVersionString, bazeliskHome, repos, config)
		if err != nil {
			return nil, fmt.Errorf("could not download Bazel: %v", err)
		}
	} else {
		// If the Bazel version is an absolute path to a Bazel binary in the filesystem, we can
		// use it directly. In that case, we don't know which exact version it is, though.
		resolvedVersion = "unknown"
		baseDirectory := filepath.Join(bazeliskHome, "local")
		bazelPath, err = linkLocalBazel(baseDirectory, bazelPath)
		if err != nil {
			return nil, fmt.Errorf("could not link local Bazel: %v", err)
		}
	}

	return &BazelInstallation{
			Version: resolvedVersion,
			Path:    bazelPath,
		},
		nil
}

func getBazelCommand(args []string) (string, error) {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a, nil
		}
	}
	return "", fmt.Errorf("could not find a valid Bazel command in %q. Please run `bazel help` if you need help on how to use Bazel", strings.Join(args, " "))
}

// getBazeliskHome returns the path to the Bazelisk home directory.
func getBazeliskHome(config config.Config) (string, error) {
	bazeliskHome := config.Get("BAZELISK_HOME_" + strings.ToUpper(runtime.GOOS))
	if len(bazeliskHome) == 0 {
		bazeliskHome = config.Get("BAZELISK_HOME")
	}

	if len(bazeliskHome) == 0 {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("could not get the user's cache directory: %v", err)
		}

		bazeliskHome = filepath.Join(userCacheDir, "bazelisk")
	} else {
		// If a custom BAZELISK_HOME is set, handle tilde and var expansion
		// before creating the Bazelisk home directory.
		var err error

		bazeliskHome, err = homedir.Expand(bazeliskHome)
		if err != nil {
			return "", fmt.Errorf("could not expand home directory in path: %v", err)
		}

		bazeliskHome = os.ExpandEnv(bazeliskHome)
	}

	return bazeliskHome, nil
}

func getUserAgent(config config.Config) string {
	agent := config.Get("BAZELISK_USER_AGENT")
	if len(agent) > 0 {
		return agent
	}
	return fmt.Sprintf("Bazelisk/%s", BazeliskVersion)
}

// GetBazelVersion returns the Bazel version that should be used.
func GetBazelVersion(config config.Config) (string, error) {
	// Check in this order:
	// - env var "USE_BAZEL_VERSION" is set to a specific version.
	// - workspace_root/.bazeliskrc exists -> read contents, in contents:
	//   var "USE_BAZEL_VERSION" is set to a specific version.
	// - env var "USE_NIGHTLY_BAZEL" or "USE_BAZEL_NIGHTLY" is set -> latest
	//   nightly. (TODO)
	// - env var "USE_CANARY_BAZEL" or "USE_BAZEL_CANARY" is set -> latest
	//   rc. (TODO)
	// - the file workspace_root/tools/bazel exists -> that version. (TODO)
	// - workspace_root/.bazelversion exists -> read contents, that version.
	// - workspace_root/WORKSPACE contains a version -> that version. (TODO)
	// - env var "USE_BAZEL_FALLBACK_VERSION" is set to a fallback version format.
	// - workspace_root/.bazeliskrc exists -> read contents, in contents:
	//   var "USE_BAZEL_FALLBACK_VERSION" is set to a fallback version format.
	// - fallback version format "silent:latest"
	bazelVersion := config.Get("USE_BAZEL_VERSION")
	if len(bazelVersion) != 0 {
		return bazelVersion, nil
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not get working directory: %v", err)
	}

	workspaceRoot := ws.FindWorkspaceRoot(workingDirectory)
	if len(workspaceRoot) != 0 {
		bazelVersionPath := filepath.Join(workspaceRoot, ".bazelversion")
		if _, err := os.Stat(bazelVersionPath); err == nil {
			f, err := os.Open(bazelVersionPath)
			if err != nil {
				return "", fmt.Errorf("could not read %s: %v", bazelVersionPath, err)
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			scanner.Scan()
			bazelVersion := scanner.Text()
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("could not read version from file %s: %v", bazelVersion, err)
			}

			if len(bazelVersion) != 0 {
				return bazelVersion, nil
			}
		}
	}

	fallbackVersionFormat := config.Get("USE_BAZEL_FALLBACK_VERSION")
	fallbackVersionMode, fallbackVersion, hasFallbackVersionMode := strings.Cut(fallbackVersionFormat, ":")
	if !hasFallbackVersionMode {
		fallbackVersionMode, fallbackVersion, hasFallbackVersionMode = "silent", fallbackVersionMode, true
	}
	if len(fallbackVersion) == 0 {
		fallbackVersion = "latest"
	}
	if fallbackVersionMode == "error" {
		return "", fmt.Errorf("not allowed to use fallback version %q", fallbackVersion)
	}
	if fallbackVersionMode == "warn" {
		log.Printf("Warning: used fallback version %q\n", fallbackVersion)
		return fallbackVersion, nil
	}
	if fallbackVersionMode == "silent" {
		return fallbackVersion, nil
	}
	return "", fmt.Errorf("invalid fallback version format %q (effectively %q)", fallbackVersionFormat, fmt.Sprintf("%s:%s", fallbackVersionMode, fallbackVersion))
}

func parseBazelForkAndVersion(bazelForkAndVersion string) (string, string, error) {
	var bazelFork, bazelVersion string

	versionInfo := strings.Split(bazelForkAndVersion, "/")

	if len(versionInfo) == 1 {
		bazelFork, bazelVersion = versions.BazelUpstream, versionInfo[0]
	} else if len(versionInfo) == 2 {
		bazelFork, bazelVersion = versionInfo[0], versionInfo[1]
	} else {
		return "", "", fmt.Errorf("invalid version %q, could not parse version with more than one slash", bazelForkAndVersion)
	}

	return bazelFork, bazelVersion, nil
}

func downloadBazel(bazelVersionString string, bazeliskHome string, repos *Repositories, config config.Config) (string, error) {
	bazelFork, bazelVersion, err := parseBazelForkAndVersion(bazelVersionString)
	if err != nil {
		return "", fmt.Errorf("could not parse Bazel fork and version: %v", err)
	}

	resolvedBazelVersion, downloader, err := repos.ResolveVersion(bazeliskHome, bazelFork, bazelVersion, config)
	if err != nil {
		return "", fmt.Errorf("could not resolve the version '%s' to an actual version number: %v", bazelVersion, err)
	}

	bazelForkOrURL := dirForURL(config.Get(BaseURLEnv))
	if len(bazelForkOrURL) == 0 {
		bazelForkOrURL = bazelFork
	}

	bazelPath, err := downloadBazelIfNecessary(resolvedBazelVersion, bazeliskHome, bazelForkOrURL, repos, config, downloader)
	return bazelPath, err
}

// downloadBazelIfNecessary returns a path to a bazel which can be run, which may have been cached.
// The directory it returns may depend on version and bazeliskHome, but does not depend on bazelForkOrURLDirName.
// This is important, as the directory may be added to $PATH, and varying the path for equivalent files may cause unnecessary repository rule cache invalidations.
// Where a file was downloaded from shouldn't affect cache behaviour of Bazel invocations.
//
// The structure of the downloads directory is as follows ([]s indicate variables):
//
//	downloads/metadata/[fork-or-url]/bazel-[version-os-etc] is a text file containing a hex sha256 of the contents of the downloaded bazel file.
//	downloads/sha256/[sha256]/bin/bazel[extension] contains the bazel with a particular sha256.
func downloadBazelIfNecessary(version string, bazeliskHome string, bazelForkOrURLDirName string, repos *Repositories, config config.Config, downloader DownloadFunc) (string, error) {
	pathSegment, err := platforms.DetermineBazelFilename(version, false, config)
	if err != nil {
		return "", fmt.Errorf("could not determine path segment to use for Bazel binary: %v", err)
	}

	destFile := "bazel" + platforms.DetermineExecutableFilenameSuffix()

	mappingPath := filepath.Join(bazeliskHome, "downloads", "metadata", bazelForkOrURLDirName, pathSegment)
	digestFromMappingFile, err := os.ReadFile(mappingPath)
	if err == nil {
		pathToBazelInCAS := filepath.Join(bazeliskHome, "downloads", "sha256", string(digestFromMappingFile), "bin", destFile)
		if _, err := os.Stat(pathToBazelInCAS); err == nil {
			return pathToBazelInCAS, nil
		}
	}

	pathToBazelInCAS, downloadedDigest, err := downloadBazelToCAS(version, bazeliskHome, repos, config, downloader)
	if err != nil {
		return "", fmt.Errorf("failed to download bazel: %w", err)
	}

	expectedSha256 := strings.ToLower(config.Get("BAZELISK_VERIFY_SHA256"))
	if len(expectedSha256) > 0 {
		if expectedSha256 != downloadedDigest {
			return "", fmt.Errorf("%s has sha256=%s but need sha256=%s", pathToBazelInCAS, downloadedDigest, expectedSha256)
		}
	}

	if err := atomicWriteFile(mappingPath, []byte(downloadedDigest), 0644); err != nil {
		return "", fmt.Errorf("failed to write mapping file after downloading bazel: %w", err)
	}

	return pathToBazelInCAS, nil
}

func atomicWriteFile(path string, contents []byte, perm os.FileMode) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("failed to MkdirAll parent of %s: %w", path, err)
	}
	tmpFile, err := os.CreateTemp(parent, filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file in %s: %w", parent, err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())
	tmpPath := tmpFile.Name()
	if err := os.WriteFile(tmpPath, contents, perm); err != nil {
		return fmt.Errorf("failed to write file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tmpPath, path, err)
	}
	return nil
}

// lockedRenameIfDstAbsent executes os.Rename under file lock to avoid issues
// of multiple bazelisk processes renaming file to the same destination file.
// See https://github.com/bazelbuild/bazelisk/issues/436.
func lockedRenameIfDstAbsent(src, dst string) error {
	lockFile := dst + ".lock"
	fileLock := flock.New(lockFile)

	// Do not wait for lock forever to avoid hanging in any scenarios. This
	// makes the lock best-effort.
	lockCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ok, err := fileLock.TryLockContext(lockCtx, 50*time.Millisecond)
	if !ok || err != nil {
		log.Printf("WARNING: Unable to create lock during rename to %s, this may cause issues for parallel bazel executions: %s\n", lockFile, err)
	} else {
		defer func() {
			_ = fileLock.Unlock()
		}()
	}

	if _, err := os.Stat(dst); err == nil {
		return nil
	}

	return os.Rename(src, dst)
}

func downloadBazelToCAS(version string, bazeliskHome string, repos *Repositories, config config.Config, downloader DownloadFunc) (string, string, error) {
	downloadsDir := filepath.Join(bazeliskHome, "downloads")
	temporaryDownloadDir := filepath.Join(downloadsDir, "_tmp")
	casDir := filepath.Join(bazeliskHome, "downloads", "sha256")

	tmpDestFileBytes := make([]byte, 32)
	if _, err := rand.Read(tmpDestFileBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate temporary file name: %w", err)
	}
	tmpDestFile := fmt.Sprintf("%x", tmpDestFileBytes)

	var tmpDestPath string
	var err error
	baseURL := config.Get(BaseURLEnv)
	formatURL := config.Get(FormatURLEnv)
	if baseURL != "" && formatURL != "" {
		return "", "", fmt.Errorf("cannot set %s and %s at once", BaseURLEnv, FormatURLEnv)
	} else if formatURL != "" {
		tmpDestPath, err = repos.DownloadFromFormatURL(config, formatURL, version, temporaryDownloadDir, tmpDestFile)
	} else if baseURL != "" {
		tmpDestPath, err = repos.DownloadFromBaseURL(baseURL, version, temporaryDownloadDir, tmpDestFile, config)
	} else {
		tmpDestPath, err = downloader(temporaryDownloadDir, tmpDestFile)
	}
	if err != nil {
		return "", "", fmt.Errorf("failed to download bazel: %w", err)
	}

	f, err := os.Open(tmpDestPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to open downloaded bazel to digest it: %w", err)
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		f.Close()
		return "", "", fmt.Errorf("cannot compute sha256 of %s after download: %v", tmpDestPath, err)
	}
	f.Close()
	actualSha256 := strings.ToLower(fmt.Sprintf("%x", h.Sum(nil)))

	bazelInCASBasename := "bazel" + platforms.DetermineExecutableFilenameSuffix()
	pathToBazelInCAS := filepath.Join(casDir, actualSha256, "bin", bazelInCASBasename)
	dirForBazelInCAS := filepath.Dir(pathToBazelInCAS)
	if err := os.MkdirAll(dirForBazelInCAS, 0755); err != nil {
		return "", "", fmt.Errorf("failed to MkdirAll parent of %s: %w", pathToBazelInCAS, err)
	}

	tmpPathFile, err := os.CreateTemp(dirForBazelInCAS, bazelInCASBasename+".tmp")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temporary file in %s: %w", dirForBazelInCAS, err)
	}
	tmpPathFile.Close()
	defer os.Remove(tmpPathFile.Name())
	tmpPathInCorrectDirectory := tmpPathFile.Name()
	if err := os.Rename(tmpDestPath, tmpPathInCorrectDirectory); err != nil {
		return "", "", fmt.Errorf("failed to move %s to %s: %w", tmpDestPath, tmpPathInCorrectDirectory, err)
	}
	if err := lockedRenameIfDstAbsent(tmpPathInCorrectDirectory, pathToBazelInCAS); err != nil {
		return "", "", fmt.Errorf("failed to move %s to %s: %w", tmpPathInCorrectDirectory, pathToBazelInCAS, err)
	}

	return pathToBazelInCAS, actualSha256, nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE, perm)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)

	return err
}

func linkLocalBazel(baseDirectory string, bazelPath string) (string, error) {
	normalizedBazelPath := dirForURL(bazelPath)
	destinationDir := filepath.Join(baseDirectory, normalizedBazelPath, "bin")
	err := os.MkdirAll(destinationDir, 0755)
	if err != nil {
		return "", fmt.Errorf("could not create directory %s: %v", destinationDir, err)
	}
	destinationPath := filepath.Join(destinationDir, "bazel"+platforms.DetermineExecutableFilenameSuffix())
	if _, err := os.Stat(destinationPath); err != nil {
		err = os.Symlink(bazelPath, destinationPath)
		// If can't create Symlink, fallback to copy
		if err != nil {
			err = copyFile(bazelPath, destinationPath, 0755)
			if err != nil {
				return "", fmt.Errorf("could not copy file from %s to %s: %v", bazelPath, destinationPath, err)
			}
		}
	}
	return destinationPath, nil
}

func maybeDelegateToWrapperFromDir(bazel string, wd string, config config.Config) string {
	if config.Get(skipWrapperEnv) != "" {
		return bazel
	}

	wrapperPath := config.Get("BAZELISK_WRAPPER_DIRECTORY")
	if wrapperPath == "" {
		wrapperPath = filepath.Join(defaultWrapperDirectory, defaultWrapperName)
	} else {
		wrapperPath = filepath.Join(wrapperPath, defaultWrapperName)
	}

	root := ws.FindWorkspaceRoot(wd)
	wrapper := filepath.Join(root, wrapperPath)
	if stat, err := os.Stat(wrapper); err == nil && !stat.Mode().IsDir() && stat.Mode().Perm()&0111 != 0 {
		return wrapper
	}

	if runtime.GOOS == "windows" {
		powershellWrapper := filepath.Join(root, wrapperPath+".ps1")
		if stat, err := os.Stat(powershellWrapper); err == nil && !stat.Mode().IsDir() {
			return powershellWrapper
		}

		batchWrapper := filepath.Join(root, wrapperPath+".bat")
		if stat, err := os.Stat(batchWrapper); err == nil && !stat.Mode().IsDir() {
			return batchWrapper
		}
	}

	return bazel
}

func maybeDelegateToWrapper(bazel string, config config.Config) string {
	wd, err := os.Getwd()
	if err != nil {
		return bazel
	}

	return maybeDelegateToWrapperFromDir(bazel, wd, config)
}

func prependDirToPathList(cmd *exec.Cmd, dir string) {
	found := false
	for idx, val := range cmd.Env {
		splits := strings.Split(val, "=")
		if len(splits) != 2 {
			continue
		}
		if strings.EqualFold(splits[0], "PATH") {
			found = true
			cmd.Env[idx] = fmt.Sprintf("PATH=%s%s%s", dir, string(os.PathListSeparator), splits[1])
			break
		}
	}

	if !found {
		cmd.Env = append(cmd.Env, fmt.Sprintf("PATH=%s", dir))
	}
}

func makeBazelCmd(bazel string, args []string, out io.Writer, config config.Config) *exec.Cmd {
	execPath := maybeDelegateToWrapper(bazel, config)

	cmd := exec.Command(execPath, args...)
	cmd.Env = append(os.Environ(), skipWrapperEnv+"=true")
	if execPath != bazel {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", bazelReal, bazel))
	}
	selfPath, err := os.Executable()
	if err != nil {
		cmd.Env = append(cmd.Env, bazeliskEnv+"="+selfPath)
	}
	prependDirToPathList(cmd, filepath.Dir(execPath))
	cmd.Stdin = os.Stdin
	if out == nil {
		cmd.Stdout = os.Stdout
	} else {
		cmd.Stdout = out
	}
	cmd.Stderr = os.Stderr
	return cmd
}

func runBazel(bazel string, args []string, out io.Writer, config config.Config) (int, error) {
	cmd := makeBazelCmd(bazel, args, out, config)
	err := cmd.Start()
	if err != nil {
		return 1, fmt.Errorf("could not start Bazel: %v", err)
	}

	// Ignore signals recognized by the Bazel client.
	// The Bazel client implements its own handling of these signals by
	// forwarding them to the Bazel server, and they don't necessarily cause
	// the invocation to be immediately aborted. In particular, SIGINT and
	// SIGTERM are handled gracefully and may cause a delayed exit, while
	// SIGQUIT requests a Java thread dump from the Bazel server, but doesn't
	// abort the invocation. Normally, these signals are delivered by the
	// terminal to the entire process group led by Bazelisk. If Bazelisk were
	// to immediately exit in response to one of these signals, it would cause
	// the still running Bazel client to become an orphan and uncontrollable
	// by the terminal. As a side effect, we also suppress the printing of a
	// Go stack trace upon receiving SIGQUIT, which is unhelpful as users tend
	// to report it instead of the far more valuable Java thread dump.
	// TODO(#512): We may want to treat a `bazel run` command differently.
	// Since signal handlers are process-wide global state and bazelisk may be
	// used as a library, reset the signal handlers after the process exits.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	err = cmd.Wait()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus := exitError.Sys().(syscall.WaitStatus)
			return waitStatus.ExitStatus(), nil
		}
		return 1, fmt.Errorf("could not launch Bazel: %v", err)
	}
	return 0, nil
}

// getIncompatibleFlags returns all incompatible flags for the current Bazel command in alphabetical order.
func getIncompatibleFlags(bazelPath, cmd string, config config.Config) ([]string, error) {
	var incompatibleFlagsStr = config.Get("BAZELISK_INCOMPATIBLE_FLAGS")
	if len(incompatibleFlagsStr) > 0 {
		return strings.Split(incompatibleFlagsStr, ","), nil
	}

	out := strings.Builder{}
	if _, err := runBazel(bazelPath, []string{"help", cmd, "--short"}, &out, config); err != nil {
		return nil, fmt.Errorf("unable to determine incompatible flags with binary %s: %v", bazelPath, err)
	}

	re := regexp.MustCompile(`(?m)^\s*--\[no\](incompatible_\w+)$`)
	flags := make([]string, 0)
	for _, m := range re.FindAllStringSubmatch(out.String(), -1) {
		flags = append(flags, fmt.Sprintf("--%s", m[1]))
	}
	sort.Strings(flags)
	return flags, nil
}

// insertArgs will insert newArgs in baseArgs. If baseArgs contains the
// "--" argument, newArgs will be inserted before that. Otherwise, newArgs
// is appended.
func insertArgs(baseArgs []string, newArgs []string) []string {
	var result []string
	inserted := false
	for _, arg := range baseArgs {
		if !inserted && arg == "--" {
			result = append(result, newArgs...)
			inserted = true
		}
		result = append(result, arg)
	}

	if !inserted {
		result = append(result, newArgs...)
	}
	return result
}

func parseStartupOptions(baseArgs []string) []string {
	var result []string
	var bazelCommands = map[string]bool{
		"analyze-profile":    true,
		"aquery":             true,
		"build":              true,
		"canonicalize-flags": true,
		"clean":              true,
		"coverage":           true,
		"cquery":             true,
		"dump":               true,
		"fetch":              true,
		"help":               true,
		"info":               true,
		"license":            true,
		"mobile-install":     true,
		"mod":                true,
		"print_action":       true,
		"query":              true,
		"run":                true,
		"shutdown":           true,
		"sync":               true,
		"test":               true,
		"version":            true,
	}
	// Arguments before a Bazel command are startup options.
	for _, arg := range baseArgs {
		if _, ok := bazelCommands[arg]; ok {
			return result
		}
		result = append(result, arg)
	}
	return result
}

func shutdownIfNeeded(bazelPath string, startupOptions []string, config config.Config) {
	bazeliskClean := config.Get("BAZELISK_SHUTDOWN")
	if len(bazeliskClean) == 0 {
		return
	}

	args := append(startupOptions, "shutdown")
	fmt.Printf("bazel %s\n", strings.Join(args, " "))
	exitCode, err := runBazel(bazelPath, args, nil, config)
	fmt.Printf("\n")
	if err != nil {
		log.Fatalf("failed to run bazel shutdown: %v", err)
	}
	if exitCode != 0 {
		fmt.Printf("Failure: shutdown command failed.\n")
		os.Exit(exitCode)
	}
}

func cleanIfNeeded(bazelPath string, startupOptions []string, config config.Config) {
	bazeliskClean := config.Get("BAZELISK_CLEAN")
	if len(bazeliskClean) == 0 {
		return
	}

	args := append(startupOptions, "clean", "--expunge")
	fmt.Printf("bazel %s\n", strings.Join(args, " "))
	exitCode, err := runBazel(bazelPath, args, nil, config)
	fmt.Printf("\n")
	if err != nil {
		log.Fatalf("failed to run clean: %v", err)
	}
	if exitCode != 0 {
		fmt.Printf("Failure: clean command failed.\n")
		os.Exit(exitCode)
	}
}

type parentCommit struct {
	SHA string `json:"sha"`
}

type commit struct {
	SHA     string         `json:"sha"`
	PARENTS []parentCommit `json:"parents"`
}

type compareResponse struct {
	Commits         []commit `json:"commits"`
	BaseCommit      commit   `json:"base_commit"`
	MergeBaseCommit commit   `json:"merge_base_commit"`
}

func sendRequest(url string, config config.Config) (*http.Response, error) {
	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	githubToken := config.Get("BAZELISK_GITHUB_TOKEN")
	if len(githubToken) != 0 {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", githubToken))
	}

	return client.Do(req)
}

func getBazelCommitsBetween(oldCommit string, newCommit string, config config.Config) (string, []string, error) {
	commitList := make([]string, 0)
	page := 1
	perPage := 250 // 250 is the maximum number of commits per page

	for {
		url := fmt.Sprintf("https://api.github.com/repos/bazelbuild/bazel/compare/%s...%s?page=%d&per_page=%d", oldCommit, newCommit, page, perPage)

		response, err := sendRequest(url, config)
		if err != nil {
			return oldCommit, nil, fmt.Errorf("Error fetching commit data: %v", err)
		}
		defer response.Body.Close()

		body, err := io.ReadAll(response.Body)
		if err != nil {
			return oldCommit, nil, fmt.Errorf("Error reading response body: %v", err)
		}

		if response.StatusCode == http.StatusNotFound {
			return oldCommit, nil, fmt.Errorf("repository or commit not found: %s", string(body))
		} else if response.StatusCode == 403 {
			return oldCommit, nil, fmt.Errorf("github API rate limit hit, consider setting BAZELISK_GITHUB_TOKEN: %s", string(body))
		} else if response.StatusCode != http.StatusOK {
			return oldCommit, nil, fmt.Errorf("unexpected response status code %d: %s", response.StatusCode, string(body))
		}

		var compResp compareResponse
		err = json.Unmarshal(body, &compResp)
		if err != nil {
			return oldCommit, nil, fmt.Errorf("Error unmarshaling JSON: %v", err)
		}

		if len(compResp.Commits) == 0 {
			break
		}

		mergeBaseCommit := compResp.MergeBaseCommit.SHA
		oldCommit = mergeBaseCommit
		if mergeBaseCommit != compResp.BaseCommit.SHA {
			fmt.Printf("The old Bazel commit is not an ancestor of the new Bazel commit, overriding the old Bazel commit to the merge base commit %s\n", mergeBaseCommit)
		}

		for _, commit := range compResp.Commits {
			// If it has only one parent commit, add it to the list, otherwise it's a merge commit and we ignore it
			if len(commit.PARENTS) == 1 {
				commitList = append(commitList, commit.SHA)
			}
		}

		// Check if there are more commits to fetch
		if len(compResp.Commits) < perPage {
			break
		}

		page++
	}

	if len(commitList) == 0 {
		return oldCommit, nil, fmt.Errorf("no commits found between (%s, %s], the old commit should be first, maybe try with --bisect=%s..%s or --bisect=~%s..%s?", oldCommit, newCommit, newCommit, oldCommit, oldCommit, newCommit)
	}
	fmt.Printf("Found %d commits between (%s, %s]\n", len(commitList), oldCommit, newCommit)
	return oldCommit, commitList, nil
}

func bisect(oldCommit string, newCommit string, args []string, bazeliskHome string, repos *Repositories, config config.Config) {
	var oldCommitIs string
	if strings.HasPrefix(oldCommit, "~") {
		oldCommit = oldCommit[1:]
		oldCommitIs = "bad"
	} else {
		oldCommitIs = "good"
	}

	// 1. Get the list of commits between oldCommit and newCommit
	fmt.Printf("\n\n--- Getting the list of commits between %s and %s\n\n", oldCommit, newCommit)
	oldCommit, commitList, err := getBazelCommitsBetween(oldCommit, newCommit, config)
	if err != nil {
		log.Fatalf("Failed to get commits: %v", err)
	}

	// 2. Check if oldCommit is actually good/bad as specified
	fmt.Printf("\n\n--- Verifying if the given %s Bazel commit (%s) is actually %s\n\n", oldCommitIs, oldCommit, oldCommitIs)
	bazelExitCode, err := testWithBazelAtCommit(oldCommit, args, bazeliskHome, repos, config)
	if err != nil {
		log.Fatalf("could not run Bazel: %v", err)
	}
	if oldCommitIs == "good" && bazelExitCode != 0 {
		fmt.Printf("Failure: Given good bazel commit is already broken.\n")
	} else if oldCommitIs == "bad" && bazelExitCode == 0 {
		fmt.Printf("Failure: Given bad bazel commit is already fixed.\n")
	}

	// 3. Bisect commits
	fmt.Printf("\n\n--- Start bisecting\n\n")
	left := 0
	right := len(commitList)
	for left < right {
		mid := (left + right) / 2
		midCommit := commitList[mid]
		fmt.Printf("\n\n--- Testing with Bazel built at %s, %d commits remaining...\n\n", midCommit, right-left)
		bazelExitCode, err := testWithBazelAtCommit(midCommit, args, bazeliskHome, repos, config)
		if err != nil {
			log.Fatalf("could not run Bazel: %v", err)
		}
		if bazelExitCode == 8 {
			// Bazel was interrupted, which most likely happened because the
			// user pressed Ctrl-C. We should stop the bisecting process.
			fmt.Printf("Bisecting was interrupted, stopping...\n")
			os.Exit(8)
		}
		if bazelExitCode == 0 {
			fmt.Printf("\n\n--- Succeeded at %s\n\n", midCommit)
			if oldCommitIs == "good" {
				left = mid + 1
			} else {
				right = mid
			}
		} else {
			fmt.Printf("\n\n--- Failed at %s\n\n", midCommit)
			if oldCommitIs == "good" {
				right = mid
			} else {
				left = mid + 1
			}
		}
	}

	// 4. Print the result
	fmt.Printf("\n\n--- Bisect Result\n\n")
	if right == len(commitList) {
		if oldCommitIs == "good" {
			fmt.Printf("first bad commit not found, every commit succeeded.\n")
		} else {
			fmt.Printf("first good commit not found, every commit failed.\n")
		}
	} else {
		flippingCommit := commitList[right]
		if oldCommitIs == "good" {
			fmt.Printf("first bad commit is https://github.com/bazelbuild/bazel/commit/%s\n", flippingCommit)
		} else {
			fmt.Printf("first good commit is https://github.com/bazelbuild/bazel/commit/%s\n", flippingCommit)
		}
	}

	os.Exit(0)
}

func testWithBazelAtCommit(bazelCommit string, args []string, bazeliskHome string, repos *Repositories, config config.Config) (int, error) {
	bazelPath, err := downloadBazel(bazelCommit, bazeliskHome, repos, config)
	if err != nil {
		return 1, fmt.Errorf("could not download Bazel: %v", err)
	}
	startupOptions := parseStartupOptions(args)
	shutdownIfNeeded(bazelPath, startupOptions, config)
	cleanIfNeeded(bazelPath, startupOptions, config)
	fmt.Printf("bazel %s\n", strings.Join(args, " "))
	bazelExitCode, err := runBazel(bazelPath, args, nil, config)
	if err != nil {
		return -1, fmt.Errorf("could not run Bazel: %v", err)
	}
	return bazelExitCode, nil
}

// migrate will run Bazel with each flag separately and report which ones are failing.
func migrate(bazelPath string, baseArgs []string, flags []string, config config.Config) {
	var startupOptions = parseStartupOptions(baseArgs)

	// 1. Try without any incompatible flags, as a sanity check.
	args := baseArgs
	fmt.Printf("\n\n--- Running Bazel with no incompatible flags\n\n")
	shutdownIfNeeded(bazelPath, startupOptions, config)
	cleanIfNeeded(bazelPath, startupOptions, config)
	fmt.Printf("bazel %s\n", strings.Join(args, " "))
	exitCode, err := runBazel(bazelPath, args, nil, config)
	if err != nil {
		log.Fatalf("could not run Bazel: %v", err)
	}
	if exitCode != 0 {
		fmt.Printf("Failure: Command failed, even without incompatible flags.\n")
		os.Exit(exitCode)
	}

	// 2. Try with all the flags.
	args = insertArgs(baseArgs, flags)
	fmt.Printf("\n\n--- Running Bazel with all incompatible flags\n\n")
	shutdownIfNeeded(bazelPath, startupOptions, config)
	cleanIfNeeded(bazelPath, startupOptions, config)
	fmt.Printf("bazel %s\n", strings.Join(args, " "))
	exitCode, err = runBazel(bazelPath, args, nil, config)
	if err != nil {
		log.Fatalf("could not run Bazel: %v", err)
	}
	if exitCode == 0 {
		fmt.Printf("Success: No migration needed.\n")
		os.Exit(0)
	}

	// 3. Try with each flag separately.
	var passList []string
	var failList []string
	for _, arg := range flags {
		args = insertArgs(baseArgs, []string{arg})
		fmt.Printf("\n\n--- Running Bazel with %s\n\n", arg)
		shutdownIfNeeded(bazelPath, startupOptions, config)
		cleanIfNeeded(bazelPath, startupOptions, config)
		fmt.Printf("bazel %s\n", strings.Join(args, " "))
		exitCode, err = runBazel(bazelPath, args, nil, config)
		if err != nil {
			log.Fatalf("could not run Bazel: %v", err)
		}
		if exitCode == 0 {
			passList = append(passList, arg)
		} else {
			failList = append(failList, arg)
		}
	}

	print := func(l []string) {
		for _, arg := range l {
			fmt.Printf("  %s\n", arg)
		}
	}

	// 4. Print report
	fmt.Printf("\n\n+++ Result\n\n")
	fmt.Printf("Command was successful with the following flags:\n")
	print(passList)
	fmt.Printf("\n")
	fmt.Printf("Migration is needed for the following flags:\n")
	print(failList)

	// Return an unique exit code for incompatible flag test failure
	os.Exit(73)
}

func dirForURL(url string) string {
	// Replace all characters that might not be allowed in filenames with "-".
	dir := regexp.MustCompile("[[:^alnum:]]").ReplaceAllString(url, "-")
	// Work around length limit on some systems by truncating and then appending
	// a sha256 hash of the URL.
	if len(dir) > maxDirLength {
		suffix := fmt.Sprintf("...%x", sha256.Sum256([]byte(url)))
		dir = dir[:maxDirLength-len(suffix)] + suffix
	}
	return dir
}

func isCompletionCommand(args []string) bool {
	for _, arg := range args {
		if arg == "completion" {
			return true
		} else if !strings.HasPrefix(arg, "--") {
			return false // First non-flag arg is not "completion"
		}
	}
	return false
}

func handleCompletionCommand(args []string, bazelInstallation *BazelInstallation, config config.Config) error {
	// Look for the shell type after "completion"
	var shell string
	foundCompletion := false
	for _, arg := range args {
		if foundCompletion {
			shell = arg
			break
		}
		if arg == "completion" {
			foundCompletion = true
		}
	}

	if shell != "bash" && shell != "fish" {
		return fmt.Errorf("only bash and fish completion are supported, got: %s", shell)
	}

	// Get bazelisk home directory
	bazeliskHome, err := getBazeliskHome(config)
	if err != nil {
		return fmt.Errorf("could not determine bazelisk home: %v", err)
	}

	// Get the completion script for the current Bazel version
	completionScript, err := getBazelCompletionScript(bazelInstallation.Version, bazeliskHome, shell, config)
	if err != nil {
		return fmt.Errorf("could not get completion script: %v", err)
	}

	fmt.Print(completionScript)
	return nil
}

func getBazelCompletionScript(version string, bazeliskHome string, shell string, config config.Config) (string, error) {
	var completionFilename string
	switch shell {
	case "bash":
		completionFilename = "bazel-complete.bash"
	case "fish":
		completionFilename = "bazel.fish"
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}

	// Construct installer URL using the same logic as bazel binary downloads
	baseURL := config.Get(BaseURLEnv)
	formatURL := config.Get(FormatURLEnv)

	installerURL, err := constructInstallerURL(baseURL, formatURL, version, config)
	if err != nil {
		return "", fmt.Errorf("could not construct installer URL: %v", err)
	}

	// Download completion scripts if necessary (handles content-based caching internally)
	installerHash, err := downloadCompletionScriptIfNecessary(installerURL, version, bazeliskHome, baseURL, config)
	if err != nil {
		return "", fmt.Errorf("could not download completion script: %v", err)
	}

	// Read the requested completion script using installer content hash
	casDir := filepath.Join(bazeliskHome, "downloads", "sha256")
	completionDir := filepath.Join(casDir, installerHash, "completion")
	requestedPath := filepath.Join(completionDir, completionFilename)
	cachedContent, err := os.ReadFile(requestedPath)
	if err != nil {
		if shell == "fish" {
			return "", fmt.Errorf("fish completion script not available for Bazel version %s", version)
		}
		return "", fmt.Errorf("could not read cached completion script: %v", err)
	}

	return string(cachedContent), nil
}

func constructInstallerURL(baseURL, formatURL, version string, config config.Config) (string, error) {
	if baseURL != "" && formatURL != "" {
		return "", fmt.Errorf("cannot set %s and %s at once", BaseURLEnv, FormatURLEnv)
	}

	if formatURL != "" {
		// Replace %v with version and construct installer-specific format
		installerFormatURL := strings.Replace(formatURL, "bazel-%v", "bazel-%v-installer", 1)
		installerFormatURL = strings.Replace(installerFormatURL, "%e", ".sh", 1)
		return BuildURLFromFormat(config, installerFormatURL, version)
	}

	if baseURL != "" {
		installerFile, err := platforms.DetermineBazelInstallerFilename(version, config)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/%s/%s", baseURL, version, installerFile), nil
	}

	// Default to GitHub
	installerFile, err := platforms.DetermineBazelInstallerFilename(version, config)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://github.com/bazelbuild/bazel/releases/download/%s/%s", version, installerFile), nil
}

func downloadCompletionScriptIfNecessary(installerURL, version, bazeliskHome, baseURL string, config config.Config) (string, error) {
	// Create installer filename for metadata mapping (similar to bazel binary)
	installerFile, err := platforms.DetermineBazelInstallerFilename(version, config)
	if err != nil {
		return "", fmt.Errorf("could not determine installer filename: %v", err)
	}

	installerForkOrURL := dirForURL(baseURL)
	if len(installerForkOrURL) == 0 {
		installerForkOrURL = "bazelbuild"
	}

	// Check metadata mapping for installer URL -> content hash
	mappingPath := filepath.Join(bazeliskHome, "downloads", "metadata", installerForkOrURL, installerFile)
	digestFromMappingFile, err := os.ReadFile(mappingPath)
	if err == nil {
		// Check if completion scripts exist for this content hash
		casDir := filepath.Join(bazeliskHome, "downloads", "sha256")
		installerHash := string(digestFromMappingFile)
		completionDir := filepath.Join(casDir, installerHash, "completion")
		bashPath := filepath.Join(completionDir, "bazel-complete.bash")

		if _, errBash := os.Stat(bashPath); errBash == nil {
			return installerHash, nil // Completion scripts already cached
		}
	}

	// Download installer and extract completion scripts
	installerHash, err := downloadInstallerToCAS(installerURL, bazeliskHome, config)
	if err != nil {
		return "", fmt.Errorf("failed to download installer: %w", err)
	}

	// Write metadata mapping
	if err := atomicWriteFile(mappingPath, []byte(installerHash), 0644); err != nil {
		return "", fmt.Errorf("failed to write mapping file: %w", err)
	}

	return installerHash, nil
}

func downloadInstallerToCAS(installerURL, bazeliskHome string, config config.Config) (string, error) {
	downloadsDir := filepath.Join(bazeliskHome, "downloads")
	temporaryDownloadDir := filepath.Join(downloadsDir, "_tmp")
	casDir := filepath.Join(bazeliskHome, "downloads", "sha256")

	// Generate temporary file name for installer download
	tmpInstallerBytes := make([]byte, 16)
	if _, err := rand.Read(tmpInstallerBytes); err != nil {
		return "", fmt.Errorf("failed to generate temporary installer file name: %w", err)
	}
	tmpInstallerFile := fmt.Sprintf("%x-installer", tmpInstallerBytes)

	// Download the installer
	installerPath, err := httputil.DownloadBinary(installerURL, temporaryDownloadDir, tmpInstallerFile, config)
	if err != nil {
		return "", fmt.Errorf("failed to download installer: %w", err)
	}
	defer os.Remove(installerPath)

	// Read installer content and compute hash
	installerContent, err := os.ReadFile(installerPath)
	if err != nil {
		return "", fmt.Errorf("failed to read installer: %w", err)
	}

	h := sha256.New()
	h.Write(installerContent)
	installerHash := strings.ToLower(fmt.Sprintf("%x", h.Sum(nil)))

	// Check if completion scripts already exist for this installer content hash
	completionDir := filepath.Join(casDir, installerHash, "completion")
	bashPath := filepath.Join(completionDir, "bazel-complete.bash")

	if _, errBash := os.Stat(bashPath); errBash == nil {
		return installerHash, nil // Completion scripts already cached
	}

	// Extract completion scripts from installer
	completionScripts, err := extractCompletionScriptsFromInstaller(installerContent)
	if err != nil {
		return "", fmt.Errorf("failed to extract completion scripts: %w", err)
	}

	// Create completion directory in CAS
	if err := os.MkdirAll(completionDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create completion directory: %w", err)
	}

	// Write completion scripts to CAS using installer content hash
	for filename, content := range completionScripts {
		scriptPath := filepath.Join(completionDir, filename)
		if err := atomicWriteFile(scriptPath, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("failed to write %s: %w", filename, err)
		}
	}

	return installerHash, nil
}

func extractCompletionScriptsFromInstaller(installerContent []byte) (map[string]string, error) {
	// Extract the zip file from the installer script
	zipData, err := extractZipFromInstaller(installerContent)
	if err != nil {
		return nil, fmt.Errorf("could not extract zip from installer: %w", err)
	}

	// Extract the completion scripts from the zip file
	completionScripts, err := extractCompletionScriptsFromZip(zipData)
	if err != nil {
		return nil, fmt.Errorf("could not extract completion scripts from zip: %w", err)
	}

	return completionScripts, nil
}

func extractZipFromInstaller(installerContent []byte) ([]byte, error) {
	// The installer script embeds a PK-formatted zip archive directly after the shell prologue.
	const zipMagic = "PK\x03\x04" // local file header signature

	idx := bytes.Index(installerContent, []byte(zipMagic))
	if idx == -1 {
		return nil, fmt.Errorf("could not find zip file in installer script")
	}

	return installerContent[idx:], nil
}

func extractCompletionScriptsFromZip(zipData []byte) (map[string]string, error) {
	// Create a zip reader from the zip data
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("could not create zip reader: %v", err)
	}

	completionScripts := make(map[string]string)
	targetFiles := map[string]string{
		"bazel-complete.bash": "bazel-complete.bash",
		"bazel.fish":          "bazel.fish",
	}

	// Look for completion files in the zip
	for _, file := range zipReader.File {
		if targetFilename, found := targetFiles[file.Name]; found {
			rc, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("could not open completion file %s: %v", file.Name, err)
			}

			completionContent, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("could not read completion file %s: %v", file.Name, err)
			}

			completionScripts[targetFilename] = string(completionContent)
		}
	}

	// Check that we found at least the bash completion script
	if _, found := completionScripts["bazel-complete.bash"]; !found {
		return nil, fmt.Errorf("bazel-complete.bash not found in zip file")
	}
	// Fish completion is optional (older Bazel versions might not have it)

	return completionScripts, nil
}
