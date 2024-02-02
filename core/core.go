// Package core contains the core Bazelisk logic, as well as abstractions for Bazel repositories.
package core

// TODO: split this file into multiple smaller ones in dedicated packages (e.g. execution, incompatible, ...).

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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
	"sync"
	"syscall"

	"github.com/bazelbuild/bazelisk/config"
	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/platforms"
	"github.com/bazelbuild/bazelisk/versions"
	"github.com/bazelbuild/bazelisk/ws"
	"github.com/mitchellh/go-homedir"
)

const (
	bazelReal      = "BAZEL_REAL"
	skipWrapperEnv = "BAZELISK_SKIP_WRAPPER"
	wrapperPath    = "./tools/bazel"
	maxDirLength   = 255
)

var (
	// BazeliskVersion is filled in via x_defs when building a release.
	BazeliskVersion = "development"

	userAgentOnce sync.Once
)

// ArgsFunc is a function that receives a resolved Bazel version and returns the arguments to invoke
// Bazel with.
type ArgsFunc func(resolvedBazelVersion string) []string

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

	userAgentOnce.Do(func() {
		httputil.UserAgent = getUserAgent(config)
	})

	bazeliskHome := config.Get("BAZELISK_HOME")
	if len(bazeliskHome) == 0 {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return -1, fmt.Errorf("could not get the user's cache directory: %v", err)
		}

		bazeliskHome = filepath.Join(userCacheDir, "bazelisk")
	}

	err := os.MkdirAll(bazeliskHome, 0755)
	if err != nil {
		return -1, fmt.Errorf("could not create directory %s: %v", bazeliskHome, err)
	}

	bazelVersionString, err := GetBazelVersion(config)
	if err != nil {
		return -1, fmt.Errorf("could not get Bazel version: %v", err)
	}

	bazelPath, err := homedir.Expand(bazelVersionString)
	if err != nil {
		return -1, fmt.Errorf("could not expand home directory in path: %v", err)
	}

	// If the Bazel version is an absolute path to a Bazel binary in the filesystem, we can
	// use it directly. In that case, we don't know which exact version it is, though.
	resolvedBazelVersion := "unknown"

	// If we aren't using a local Bazel binary, we'll have to parse the version string and
	// download the version that the user wants.
	if !filepath.IsAbs(bazelPath) {
		bazelPath, err = downloadBazel(bazelVersionString, bazeliskHome, repos, config)
		if err != nil {
			return -1, fmt.Errorf("could not download Bazel: %v", err)
		}
	} else {
		baseDirectory := filepath.Join(bazeliskHome, "local")
		bazelPath, err = linkLocalBazel(baseDirectory, bazelPath)
		if err != nil {
			return -1, fmt.Errorf("could not link local Bazel: %v", err)
		}
	}

	args := argsFunc(resolvedBazelVersion)

	// --print_env must be the first argument.
	if len(args) > 0 && args[0] == "--print_env" {
		// print environment variables for sub-processes
		cmd := makeBazelCmd(bazelPath, args, nil, config)
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
		newFlags, err := getIncompatibleFlags(bazelPath, cmd, config)
		if err != nil {
			return -1, fmt.Errorf("could not get the list of incompatible flags: %v", err)
		}
		if args[0] == "--migrate" {
			migrate(bazelPath, args[1:], newFlags, config)
		} else {
			// When --strict is present, it expands to the list of --incompatible_ flags
			// that should be enabled for the given Bazel version.
			args = insertArgs(args[1:], newFlags)
		}
	} else if len(args) > 0 && strings.HasPrefix(args[0], "--bisect") {
		// When --bisect is present, we run the bisect logic.
		if !strings.HasPrefix(args[0], "--bisect=") {
			return -1, fmt.Errorf("Error: --bisect must have a value. Expected format: '--bisect=<good bazel commit>..<bad bazel commit>'")
		}
		value := args[0][len("--bisect="):]
		commits := strings.Split(value, "..")
		if len(commits) == 2 {
			bisect(commits[0], commits[1], args[1:], bazeliskHome, repos, config)
		} else {
			return -1, fmt.Errorf("Error: Invalid format for --bisect. Expected format: '--bisect=<good bazel commit>..<bad bazel commit>'")
		}
	}

	// print bazelisk version information if "version" is the first argument
	// bazel version is executed after this command
	if len(args) > 0 && args[0] == "version" {
		// Check if the --gnu_format flag is set, if that is the case,
		// the version is printed differently
		var gnuFormat bool
		for _, arg := range args {
			if arg == "--gnu_format" {
				gnuFormat = true
				break
			}
		}

		if gnuFormat {
			fmt.Printf("Bazelisk %s\n", BazeliskVersion)
		} else {
			fmt.Printf("Bazelisk version: %s\n", BazeliskVersion)
		}
	}

	exitCode, err := runBazel(bazelPath, args, nil, config)
	if err != nil {
		return -1, fmt.Errorf("could not run Bazel: %v", err)
	}
	return exitCode, nil
}

func getBazelCommand(args []string) (string, error) {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a, nil
		}
	}
	return "", fmt.Errorf("could not find a valid Bazel command in %q. Please run `bazel help` if you need help on how to use Bazel", strings.Join(args, " "))
}

func getUserAgent(config config.Config) string {
	agent := config.Get("BAZELISK_USER_AGENT")
	if len(agent) > 0 {
		return agent
	}
	return fmt.Sprintf("Bazelisk/%s", BazeliskVersion)
}

// TODO(go 1.18): remove backport of strings.Cut
func cutString(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

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
	fallbackVersionMode, fallbackVersion, hasFallbackVersionMode := cutString(fallbackVersionFormat, ":")
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
		return "", "", fmt.Errorf("invalid version \"%s\", could not parse version with more than one slash", bazelForkAndVersion)
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
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to MkdirAll parent of %s: %w", path, err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, contents, perm); err != nil {
		return fmt.Errorf("failed to write file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tmpPath, path, err)
	}
	return nil
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

	pathToBazelInCAS := filepath.Join(casDir, actualSha256, "bin", "bazel"+platforms.DetermineExecutableFilenameSuffix())
	if err := os.MkdirAll(filepath.Dir(pathToBazelInCAS), 0755); err != nil {
		return "", "", fmt.Errorf("failed to MkdirAll parent of %s: %w", pathToBazelInCAS, err)
	}

	tmpPathInCorrectDirectory := pathToBazelInCAS + ".tmp"
	if err := os.Rename(tmpDestPath, tmpPathInCorrectDirectory); err != nil {
		return "", "", fmt.Errorf("failed to move %s to %s: %w", tmpDestPath, tmpPathInCorrectDirectory, err)
	}
	if err := os.Rename(tmpPathInCorrectDirectory, pathToBazelInCAS); err != nil {
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

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-c

		// Only forward SIGTERM to our child process.
		if s != os.Interrupt {
			cmd.Process.Kill()
		}
	}()

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

type ParentCommit struct {
	SHA string `json:"sha"`
}

type Commit struct {
	SHA     string         `json:"sha"`
	PARENTS []ParentCommit `json:"parents"`
}

type CompareResponse struct {
	Commits         []Commit `json:"commits"`
	BaseCommit      Commit   `json:"base_commit"`
	MergeBaseCommit Commit   `json:"merge_base_commit"`
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

func getBazelCommitsBetween(goodCommit string, badCommit string, config config.Config) (string, []string, error) {
	commitList := make([]string, 0)
	page := 1
	perPage := 250 // 250 is the maximum number of commits per page

	for {
		url := fmt.Sprintf("https://api.github.com/repos/bazelbuild/bazel/compare/%s...%s?page=%d&per_page=%d", goodCommit, badCommit, page, perPage)

		response, err := sendRequest(url, config)
		if err != nil {
			return goodCommit, nil, fmt.Errorf("Error fetching commit data: %v", err)
		}
		defer response.Body.Close()

		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return goodCommit, nil, fmt.Errorf("Error reading response body: %v", err)
		}

		if response.StatusCode == http.StatusNotFound {
			return goodCommit, nil, fmt.Errorf("repository or commit not found: %s", string(body))
		} else if response.StatusCode == 403 {
			return goodCommit, nil, fmt.Errorf("github API rate limit hit, consider setting BAZELISK_GITHUB_TOKEN: %s", string(body))
		} else if response.StatusCode != http.StatusOK {
			return goodCommit, nil, fmt.Errorf("unexpected response status code %d: %s", response.StatusCode, string(body))
		}

		var compareResponse CompareResponse
		err = json.Unmarshal(body, &compareResponse)
		if err != nil {
			return goodCommit, nil, fmt.Errorf("Error unmarshaling JSON: %v", err)
		}

		if len(compareResponse.Commits) == 0 {
			break
		}

		mergeBaseCommit := compareResponse.MergeBaseCommit.SHA
		if mergeBaseCommit != compareResponse.BaseCommit.SHA {
			fmt.Printf("The good Bazel commit is not an ancestor of the bad Bazel commit, overriding the good Bazel commit to the merge base commit %s\n", mergeBaseCommit)
			goodCommit = mergeBaseCommit
		}

		for _, commit := range compareResponse.Commits {
			// If it has only one parent commit, add it to the list, otherwise it's a merge commit and we ignore it
			if len(commit.PARENTS) == 1 {
				commitList = append(commitList, commit.SHA)
			}
		}

		// Check if there are more commits to fetch
		if len(compareResponse.Commits) < perPage {
			break
		}

		page++
	}

	if len(commitList) == 0 {
		return goodCommit, nil, fmt.Errorf("no commits found between (%s, %s], the good commit should be first, maybe try with --bisect=%s..%s ?", goodCommit, badCommit, badCommit, goodCommit)
	}
	fmt.Printf("Found %d commits between (%s, %s]\n", len(commitList), goodCommit, badCommit)
	return goodCommit, commitList, nil
}

func bisect(goodCommit string, badCommit string, args []string, bazeliskHome string, repos *Repositories, config config.Config) {

	// 1. Get the list of commits between goodCommit and badCommit
	fmt.Printf("\n\n--- Getting the list of commits between %s and %s\n\n", goodCommit, badCommit)
	goodCommit, commitList, err := getBazelCommitsBetween(goodCommit, badCommit, config)
	if err != nil {
		log.Fatalf("Failed to get commits: %v", err)
		os.Exit(1)
	}

	// 2. Check if goodCommit is actually good
	fmt.Printf("\n\n--- Verifying if the given good Bazel commit (%s) is actually good\n\n", goodCommit)
	bazelExitCode, err := testWithBazelAtCommit(goodCommit, args, bazeliskHome, repos, config)
	if err != nil {
		log.Fatalf("could not run Bazel: %v", err)
		os.Exit(1)
	}
	if bazelExitCode != 0 {
		fmt.Printf("Failure: Given good bazel commit is already broken.\n")
		os.Exit(1)
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
			os.Exit(1)
		}
		if bazelExitCode == 0 {
			fmt.Printf("\n\n--- Succeeded at %s\n\n", midCommit)
			left = mid + 1
		} else {
			fmt.Printf("\n\n--- Failed at %s\n\n", midCommit)
			right = mid
		}
	}

	// 4. Print the result
	fmt.Printf("\n\n--- Bisect Result\n\n")
	if right == len(commitList) {
		fmt.Printf("first bad commit not found, every commit succeeded.\n")
	} else {
		firstBadCommit := commitList[right]
		fmt.Printf("first bad commit is https://github.com/bazelbuild/bazel/commit/%s\n", firstBadCommit)
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

	// 1. Try with all the flags.
	args := insertArgs(baseArgs, flags)
	fmt.Printf("\n\n--- Running Bazel with all incompatible flags\n\n")
	shutdownIfNeeded(bazelPath, startupOptions, config)
	cleanIfNeeded(bazelPath, startupOptions, config)
	fmt.Printf("bazel %s\n", strings.Join(args, " "))
	exitCode, err := runBazel(bazelPath, args, nil, config)
	if err != nil {
		log.Fatalf("could not run Bazel: %v", err)
	}
	if exitCode == 0 {
		fmt.Printf("Success: No migration needed.\n")
		os.Exit(0)
	}

	// 2. Try with no flags, as a sanity check.
	args = baseArgs
	fmt.Printf("\n\n--- Running Bazel with no incompatible flags\n\n")
	shutdownIfNeeded(bazelPath, startupOptions, config)
	cleanIfNeeded(bazelPath, startupOptions, config)
	fmt.Printf("bazel %s\n", strings.Join(args, " "))
	exitCode, err = runBazel(bazelPath, args, nil, config)
	if err != nil {
		log.Fatalf("could not run Bazel: %v", err)
	}
	if exitCode != 0 {
		fmt.Printf("Failure: Command failed, even without incompatible flags.\n")
		os.Exit(exitCode)
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

	os.Exit(1)
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
