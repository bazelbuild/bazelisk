// Package core contains the core Bazelisk logic, as well as abstractions for Bazel repositories.
package core

// TODO: split this file into multiple smaller ones in dedicated packages (e.g. execution, incompatible, ...).

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
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

	"github.com/bazelbuild/bazelisk/httputil"
	"github.com/bazelbuild/bazelisk/platforms"
	"github.com/bazelbuild/bazelisk/versions"
	"github.com/mitchellh/go-homedir"
)

const (
	bazelReal      = "BAZEL_REAL"
	skipWrapperEnv = "BAZELISK_SKIP_WRAPPER"
	wrapperPath    = "./tools/bazel"
)

var (
	// BazeliskVersion is filled in via x_defs when building a release.
	BazeliskVersion = "development"

	fileConfig     map[string]string
	fileConfigOnce sync.Once
)

// RunBazelisk runs the main Bazelisk logic for the given arguments and Bazel repositories.
func RunBazelisk(args []string, repos *Repositories) (int, error) {
	httputil.UserAgent = getUserAgent()

	bazeliskHome := GetEnvOrConfig("BAZELISK_HOME")
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

	bazelVersionString, err := getBazelVersion()
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
		bazelFork, bazelVersion, err := parseBazelForkAndVersion(bazelVersionString)
		if err != nil {
			return -1, fmt.Errorf("could not parse Bazel fork and version: %v", err)
		}

		var downloader DownloadFunc
		resolvedBazelVersion, downloader, err = repos.ResolveVersion(bazeliskHome, bazelFork, bazelVersion)
		if err != nil {
			return -1, fmt.Errorf("could not resolve the version '%s' to an actual version number: %v", bazelVersion, err)
		}

		bazelForkOrURL := dirForURL(GetEnvOrConfig(BaseURLEnv))
		if len(bazelForkOrURL) == 0 {
			bazelForkOrURL = bazelFork
		}

		baseDirectory := filepath.Join(bazeliskHome, "downloads", bazelForkOrURL)
		bazelPath, err = downloadBazel(bazelFork, resolvedBazelVersion, baseDirectory, repos, downloader)
		if err != nil {
			return -1, fmt.Errorf("could not download Bazel: %v", err)
		}
	} else {
		baseDirectory := filepath.Join(bazeliskHome, "local")
		bazelPath, err = linkLocalBazel(baseDirectory, bazelPath)
		if err != nil {
			return -1, fmt.Errorf("cound not link local Bazel: %v", err)
		}
	}

	// --print_env must be the first argument.
	if len(args) > 0 && args[0] == "--print_env" {
		// print environment variables for sub-processes
		cmd := makeBazelCmd(bazelPath, args, nil)
		for _, val := range cmd.Env {
			fmt.Println(val)
		}
		return 0, nil
	}

	// --strict and --migrate must be the first argument.
	if len(args) > 0 && (args[0] == "--strict" || args[0] == "--migrate") {
		cmd, err := getBazelCommand(args)
		if err != nil {
			return -1, err
		}
		newFlags, err := getIncompatibleFlags(bazelPath, cmd)
		if err != nil {
			return -1, fmt.Errorf("could not get the list of incompatible flags: %v", err)
		}

		if args[0] == "--migrate" {
			migrate(bazelPath, args[1:], newFlags)
		} else {
			// When --strict is present, it expands to the list of --incompatible_ flags
			// that should be enabled for the given Bazel version.
			args = insertArgs(args[1:], newFlags)
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

	exitCode, err := runBazel(bazelPath, args, nil)
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
	return "", fmt.Errorf("could not find a valid Bazel command in %q. Please run `bazel help` if you need help on how to use Bazel.", strings.Join(args, " "))
}

func getUserAgent() string {
	agent := GetEnvOrConfig("BAZELISK_USER_AGENT")
	if len(agent) > 0 {
		return agent
	}
	return fmt.Sprintf("Bazelisk/%s", BazeliskVersion)
}

// GetEnvOrConfig reads a configuration value from the environment, but fall back to reading it from .bazeliskrc in the workspace root.
func GetEnvOrConfig(name string) string {
	if val := os.Getenv(name); val != "" {
		return val
	}

	// Parse .bazeliskrc in the workspace root, once, if it can be found.
	fileConfigOnce.Do(func() {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return
		}
		workspaceRoot := findWorkspaceRoot(workingDirectory)
		if workspaceRoot == "" {
			return
		}
		rcFilePath := filepath.Join(workspaceRoot, ".bazeliskrc")
		contents, err := ioutil.ReadFile(rcFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			log.Fatal(err)
		}
		fileConfig = make(map[string]string)
		for _, line := range strings.Split(string(contents), "\n") {
			if strings.HasPrefix(line, "#") {
				// comments
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) < 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			fileConfig[key] = strings.TrimSpace(parts[1])
		}
	})

	return fileConfig[name]
}

// isValidWorkspace returns true iff the supplied path is the workspace root, defined by the presence of
// a file named WORKSPACE or WORKSPACE.bazel
// see https://github.com/bazelbuild/bazel/blob/8346ea4cfdd9fbd170d51a528fee26f912dad2d5/src/main/cpp/workspace_layout.cc#L37
func isValidWorkspace(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	
	return !info.IsDir()
}

func findWorkspaceRoot(root string) string {
	if isValidWorkspace(filepath.Join(root, "WORKSPACE")) {
		return root
	}

	if isValidWorkspace(filepath.Join(root, "WORKSPACE.bazel")) {
		return root
	}

	parentDirectory := filepath.Dir(root)
	if parentDirectory == root {
		return ""
	}

	return findWorkspaceRoot(parentDirectory)
}

func getBazelVersion() (string, error) {
	// Check in this order:
	// - env var "USE_BAZEL_VERSION" is set to a specific version.
	// - env var "USE_NIGHTLY_BAZEL" or "USE_BAZEL_NIGHTLY" is set -> latest
	//   nightly. (TODO)
	// - env var "USE_CANARY_BAZEL" or "USE_BAZEL_CANARY" is set -> latest
	//   rc. (TODO)
	// - the file workspace_root/tools/bazel exists -> that version. (TODO)
	// - workspace_root/.bazeliskrc exists and contains a 'USE_BAZEL_VERSION'
	//   variable -> read contents, that version.
	// - workspace_root/.bazelversion exists -> read contents, that version.
	// - workspace_root/WORKSPACE contains a version -> that version. (TODO)
	// - fallback: latest release
	bazelVersion := GetEnvOrConfig("USE_BAZEL_VERSION")
	if len(bazelVersion) != 0 {
		return bazelVersion, nil
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not get working directory: %v", err)
	}

	workspaceRoot := findWorkspaceRoot(workingDirectory)
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

	return "latest", nil
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

func downloadBazel(fork string, version string, baseDirectory string, repos *Repositories, downloader DownloadFunc) (string, error) {
	pathSegment, err := platforms.DetermineBazelFilename(version, false)
	if err != nil {
		return "", fmt.Errorf("could not determine path segment to use for Bazel binary: %v", err)
	}

	destFile := "bazel" + platforms.DetermineExecutableFilenameSuffix()
	destinationDir := filepath.Join(baseDirectory, pathSegment, "bin")

	if url := GetEnvOrConfig(BaseURLEnv); url != "" {
		return repos.DownloadFromBaseURL(url, version, destinationDir, destFile)
	}

	return downloader(destinationDir, destFile)
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
				return "", fmt.Errorf("cound not copy file from %s to %s: %v", bazelPath, destinationPath, err)
			}
		}
	}
	return destinationPath, nil
}

func maybeDelegateToWrapper(bazel string) string {
	if GetEnvOrConfig(skipWrapperEnv) != "" {
		return bazel
	}

	wd, err := os.Getwd()
	if err != nil {
		return bazel
	}

	root := findWorkspaceRoot(wd)
	wrapper := filepath.Join(root, wrapperPath)
	if stat, err := os.Stat(wrapper); err != nil || stat.IsDir() || stat.Mode().Perm()&0001 == 0 {
		return bazel
	}

	return wrapper
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

func makeBazelCmd(bazel string, args []string, out io.Writer) *exec.Cmd {
	execPath := maybeDelegateToWrapper(bazel)

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

func runBazel(bazel string, args []string, out io.Writer) (int, error) {
	cmd := makeBazelCmd(bazel, args, out)
	err := cmd.Start()
	if err != nil {
		return 1, fmt.Errorf("could not start Bazel: %v", err)
	}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-c
		if runtime.GOOS != "windows" {
			cmd.Process.Signal(s)
		} else {
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
func getIncompatibleFlags(bazelPath, cmd string) ([]string, error) {
	out := strings.Builder{}
	if _, err := runBazel(bazelPath, []string{"help", cmd, "--short"}, &out); err != nil {
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

func shutdownIfNeeded(bazelPath string) {
	bazeliskClean := GetEnvOrConfig("BAZELISK_SHUTDOWN")
	if len(bazeliskClean) == 0 {
		return
	}

	fmt.Printf("bazel shutdown\n")
	exitCode, err := runBazel(bazelPath, []string{"shutdown"}, nil)
	fmt.Printf("\n")
	if err != nil {
		log.Fatalf("failed to run bazel shutdown: %v", err)
	}
	if exitCode != 0 {
		fmt.Printf("Failure: shutdown command failed.\n")
		os.Exit(exitCode)
	}
}

func cleanIfNeeded(bazelPath string) {
	bazeliskClean := GetEnvOrConfig("BAZELISK_CLEAN")
	if len(bazeliskClean) == 0 {
		return
	}

	fmt.Printf("bazel clean --expunge\n")
	exitCode, err := runBazel(bazelPath, []string{"clean", "--expunge"}, nil)
	fmt.Printf("\n")
	if err != nil {
		log.Fatalf("failed to run clean: %v", err)
	}
	if exitCode != 0 {
		fmt.Printf("Failure: clean command failed.\n")
		os.Exit(exitCode)
	}
}

// migrate will run Bazel with each flag separately and report which ones are failing.
func migrate(bazelPath string, baseArgs []string, flags []string) {
	// 1. Try with all the flags.
	args := insertArgs(baseArgs, flags)
	fmt.Printf("\n\n--- Running Bazel with all incompatible flags\n\n")
	shutdownIfNeeded(bazelPath)
	cleanIfNeeded(bazelPath)
	fmt.Printf("bazel %s\n", strings.Join(args, " "))
	exitCode, err := runBazel(bazelPath, args, nil)
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
	shutdownIfNeeded(bazelPath)
	cleanIfNeeded(bazelPath)
	fmt.Printf("bazel %s\n", strings.Join(args, " "))
	exitCode, err = runBazel(bazelPath, args, nil)
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
		shutdownIfNeeded(bazelPath)
		cleanIfNeeded(bazelPath)
		fmt.Printf("bazel %s\n", strings.Join(args, " "))
		exitCode, err = runBazel(bazelPath, args, nil)
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
	return regexp.MustCompile("[[:^alnum:]]").ReplaceAllString(url, "-")
}
