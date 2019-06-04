// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
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
	"strconv"
	"strings"
	"syscall"
	"time"

	version "github.com/hashicorp/go-version"
)

const (
	bazelReal      = "BAZEL_REAL"
	skipWrapperEnv = "BAZELISK_SKIP_WRAPPER"
	wrapperPath    = "./tools/bazel"
)

var (
	BazeliskVersion = "development"
)

func findWorkspaceRoot(root string) string {
	if _, err := os.Stat(filepath.Join(root, "WORKSPACE")); err == nil {
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
	// - workspace_root/.bazelversion exists -> read contents, that version.
	// - workspace_root/WORKSPACE contains a version -> that version. (TODO)
	// - fallback: latest release
	bazelVersion := os.Getenv("USE_BAZEL_VERSION")
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

			return bazelVersion, nil
		}
	}

	return "latest", nil
}

type release struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
}

func readRemoteFile(url string, token string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create request: %v", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not fetch %s: %v", url, err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code while reading %s: %v", url, res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read content at %s: %v", url, err)
	}
	return body, nil
}

// maybeDownload will download a file from the given url and cache the result under bazeliskHome.
// It skips the download if the file already exists and is not outdated.
// description is used only to provide better error messages.
func maybeDownload(bazeliskHome, url, filename, description string) ([]byte, error) {
	cachePath := filepath.Join(bazeliskHome, filename)

	if cacheStat, err := os.Stat(cachePath); err == nil {
		if time.Since(cacheStat.ModTime()).Hours() < 1 {
			res, err := ioutil.ReadFile(cachePath)
			if err != nil {
				return nil, fmt.Errorf("could not read %s: %v", cachePath, err)
			}
			return res, nil
		}
	}

	// We could also use go-github here, but I can't get it to build with Bazel's rules_go and it pulls in a lot of dependencies.
	body, err := readRemoteFile(url, os.Getenv("BAZELISK_GITHUB_TOKEN"))
	if err != nil {
		return nil, fmt.Errorf("could not download %s: %v", description, err)
	}

	err = ioutil.WriteFile(cachePath, body, 0666)
	if err != nil {
		return nil, fmt.Errorf("could not create %s: %v", cachePath, err)
	}

	return body, nil
}

func resolveLatestVersion(bazeliskHome string, offset int) (string, error) {
	url := "https://api.github.com/repos/bazelbuild/bazel/releases"
	releasesJSON, err := maybeDownload(bazeliskHome, url, "releases.json", "list of Bazel releases from GitHub")
	if err != nil {
		return "", fmt.Errorf("could not get releases from GitHub: %v", err)
	}

	var releases []release
	if err := json.Unmarshal(releasesJSON, &releases); err != nil {
		return "", fmt.Errorf("could not parse JSON into list of releases: %v", err)
	}

	var tags []string
	for _, release := range releases {
		if release.Prerelease {
			continue
		}
		tags = append(tags, release.TagName)
	}
	return getNthMostRecentVersion(tags, offset)
}

func getNthMostRecentVersion(versions []string, offset int) (string, error) {
	if offset >= len(versions) {
		return "", fmt.Errorf("cannot resolve version \"latest-%d\": There are only %d Bazel versions", offset, len(versions))
	}

	wrappers := make([]*version.Version, len(versions))
	for i, v := range versions {
		wrapper, err := version.NewVersion(v)
		if err != nil {
			log.Printf("WARN: Could not parse version: %s", v)
		}
		wrappers[i] = wrapper
	}
	sort.Sort(version.Collection(wrappers))
	return wrappers[len(wrappers)-1-offset].Original(), nil
}

type gcsListResponse struct {
	Prefixes []string `json:"prefixes"`
}

func resolveLatestRcVersion() (string, error) {
	versions, err := listDirectoriesInReleaseBucket("")
	if err != nil {
		return "", fmt.Errorf("could not list Bazel versions in GCS bucket: %v", err)
	}

	latestVersion, err := getHighestBazelVersion(versions)
	if err != nil {
		return "", fmt.Errorf("got invalid version number: %v", err)
	}

	// Append slash to match directories
	rcVersions, err := listDirectoriesInReleaseBucket(latestVersion + "/")
	if err != nil {
		return "", fmt.Errorf("could not list release candidates for latest release: %v", err)
	}
	return getHighestRcVersion(rcVersions)
}

func listDirectoriesInReleaseBucket(prefix string) ([]string, error) {
	url := "https://www.googleapis.com/storage/v1/b/bazel/o?delimiter=/"
	if prefix != "" {
		url = fmt.Sprintf("%s&prefix=%s", url, prefix)
	}
	content, err := readRemoteFile(url, "")
	if err != nil {
		return nil, fmt.Errorf("could not list GCS objects at %s: %v", url, err)
	}

	var response gcsListResponse
	if err := json.Unmarshal(content, &response); err != nil {
		return nil, fmt.Errorf("could not parse GCS index JSON: %v", err)
	}
	return response.Prefixes, nil
}

func getHighestBazelVersion(versions []string) (string, error) {
	for i, v := range versions {
		versions[i] = strings.TrimSuffix(v, "/")
	}
	return getNthMostRecentVersion(versions, 0)
}

func getHighestRcVersion(versions []string) (string, error) {
	var version string
	var lastRc int
	re := regexp.MustCompile(`(\d+.\d+.\d+)/rc(\d+)/`)
	for _, v := range versions {
		// Fallback: use latest release if there is no active RC.
		if strings.Index(v, "release") > -1 {
			return strings.Split(v, "/")[0], nil
		}

		m := re.FindStringSubmatch(v)
		version = m[1]
		rc, err := strconv.Atoi(m[2])
		if err != nil {
			return "", fmt.Errorf("Invalid version number %s: %v", strings.TrimSuffix(v, "/"), err)
		}
		if rc > lastRc {
			lastRc = rc
		}
	}
	return fmt.Sprintf("%src%d", version, lastRc), nil
}

func resolveVersionLabel(bazeliskHome, bazelVersion string) (string, bool, error) {
	// Returns three values:
	// 1. The label of a Blaze release (if the label resolves to a release) or a commit (for unreleased binaries),
	// 2. Whether the first value refers to a commit,
	// 3. An error.
	lastGreenCommitPathSuffixes := map[string]string{"last_green": "github.com/bazelbuild/bazel.git/bazel-bazel", "last_downstream_green": "downstream_pipeline"}
	if pathSuffix, ok := lastGreenCommitPathSuffixes[bazelVersion]; ok {
		commit, err := getLastGreenCommit(pathSuffix)
		if err != nil {
			return "", false, fmt.Errorf("cannot resolve last green commit: %v", err)
		}

		return commit, true, nil
	}

	if bazelVersion == "last_rc" {
		version, err := resolveLatestRcVersion()
		return version, false, err
	}

	r := regexp.MustCompile(`^latest(?:-(?P<offset>\d+))?$`)

	match := r.FindStringSubmatch(bazelVersion)
	if match != nil {
		offset := 0
		if match[1] != "" {
			var err error
			offset, err = strconv.Atoi(match[1])
			if err != nil {
				return "", false, fmt.Errorf("invalid version \"%s\", could not parse offset: %v", bazelVersion, err)
			}
		}
		version, err := resolveLatestVersion(bazeliskHome, offset)
		return version, false, err
	}

	return bazelVersion, false, nil
}

const lastGreenBasePath = "https://storage.googleapis.com/bazel-untrusted-builds/last_green_commit/"

func getLastGreenCommit(pathSuffix string) (string, error) {
	content, err := readRemoteFile(lastGreenBasePath+pathSuffix, "")
	if err != nil {
		return "", fmt.Errorf("could not determine last green commit: %v", err)
	}
	return strings.TrimSpace(string(content)), nil
}

func determineBazelFilename(version string) (string, error) {
	var machineName string
	switch runtime.GOARCH {
	case "amd64":
		machineName = "x86_64"
	default:
		return "", fmt.Errorf("unsupported machine architecture \"%s\", must be x86_64", runtime.GOARCH)
	}

	var osName string
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		osName = runtime.GOOS
	default:
		return "", fmt.Errorf("unsupported operating system \"%s\", must be Linux, macOS or Windows", runtime.GOOS)
	}

	filenameSuffix := ""
	if runtime.GOOS == "windows" {
		filenameSuffix = ".exe"
	}

	return fmt.Sprintf("bazel-%s-%s-%s%s", version, osName, machineName, filenameSuffix), nil
}

func determineURL(version string, isCommit bool, filename string) string {
	if isCommit {
		var platforms = map[string]string{"darwin": "macos", "linux": "ubuntu1404", "windows": "windows"}
		// No need to check the OS thanks to determineBazelFilename().
		log.Printf("Using unreleased version at commit %s", version)
		return fmt.Sprintf("https://storage.googleapis.com/bazel-builds/artifacts/%s/%s/bazel", platforms[runtime.GOOS], version)
	}

	kind := "release"
	if strings.Contains(version, "rc") {
		versionComponents := strings.Split(version, "rc")
		// Replace version with the part before rc
		version = versionComponents[0]
		kind = "rc" + versionComponents[1]
	}

	return fmt.Sprintf("https://releases.bazel.build/%s/%s/%s", version, kind, filename)
}

func downloadBazel(version string, isCommit bool, directory string) (string, error) {
	filename, err := determineBazelFilename(version)
	if err != nil {
		return "", fmt.Errorf("could not determine filename to use for Bazel binary: %v", err)
	}

	url := determineURL(version, isCommit, filename)
	destinationPath := filepath.Join(directory, filename)

	if _, err := os.Stat(destinationPath); err != nil {
		tmpfile, err := ioutil.TempFile(directory, "download")
		if err != nil {
			return "", fmt.Errorf("could not create temporary file: %v", err)
		}
		defer func() {
			err := tmpfile.Close()
			if err == nil {
				os.Remove(tmpfile.Name())
			}
		}()

		log.Printf("Downloading %s...", url)
		resp, err := http.Get(url)
		if err != nil {
			return "", fmt.Errorf("HTTP GET %s failed: %v", url, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return "", fmt.Errorf("HTTP GET %s failed with error %v", url, resp.StatusCode)
		}

		_, err = io.Copy(tmpfile, resp.Body)
		if err != nil {
			return "", fmt.Errorf("could not copy from %s to %s: %v", url, tmpfile.Name(), err)
		}

		err = os.Chmod(tmpfile.Name(), 0755)
		if err != nil {
			return "", fmt.Errorf("could not chmod file %s: %v", tmpfile.Name(), err)
		}

		tmpfile.Close()
		err = os.Rename(tmpfile.Name(), destinationPath)
		if err != nil {
			return "", fmt.Errorf("could not move %s to %s: %v", tmpfile.Name(), destinationPath, err)
		}
	}

	return destinationPath, nil
}

func maybeDelegateToWrapper(bazel string) string {
	if os.Getenv(skipWrapperEnv) != "" {
		return bazel
	}

	wd, err := os.Getwd()
	if err != nil {
		return bazel
	}

	root := findWorkspaceRoot(wd)
	wrapper := filepath.Join(root, wrapperPath)
	if stat, err := os.Stat(wrapper); err != nil || stat.Mode().Perm()&0001 == 0 {
		return bazel
	}

	return wrapper
}

func runBazel(bazel string, args []string) (int, error) {
	execPath := maybeDelegateToWrapper(bazel)
	if execPath != bazel {
		os.Setenv(bazelReal, bazel)
	}

	cmd := exec.Command(execPath, args...)
	cmd.Env = append(os.Environ(), skipWrapperEnv+"=true")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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

type issue struct {
	Title string `json:"title"`
}

type issueList struct {
	Items []issue `json:"items"`
}

func getIncompatibleFlags(bazeliskHome, resolvedBazelVersion string) ([]string, error) {
	var result []string
	// GitHub labels use only major and minor version, we ignore the patch number (and any other suffix).
	re := regexp.MustCompile(`^\d+\.\d+`)
	version := re.FindString(resolvedBazelVersion)
	if len(version) == 0 {
		return nil, fmt.Errorf("invalid version %v", resolvedBazelVersion)
	}
	url := "https://api.github.com/search/issues?per_page=100&q=repo:bazelbuild/bazel+label:migration-" + version
	issuesJSON, err := maybeDownload(bazeliskHome, url, "flags-"+version, "list of flags from GitHub")
	if err != nil {
		return nil, fmt.Errorf("could not get issues from GitHub: %v", err)
	}

	var issueList issueList
	if err := json.Unmarshal(issuesJSON, &issueList); err != nil {
		return nil, fmt.Errorf("could not parse JSON into list of issues: %v", err)
	}

	re = regexp.MustCompile(`^incompatible_\w+`)
	for _, issue := range issueList.Items {
		flag := re.FindString(issue.Title)
		if len(flag) > 0 {
			result = append(result, "--"+flag)
		}
	}

	sort.Strings(result)

	return result, nil
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

// migrate will run Bazel with each newArgs separately and report which ones are failing.
func migrate(bazelPath string, baseArgs []string, newArgs []string) {
	// 1. Try with all the flags.
	args := insertArgs(baseArgs, newArgs)
	fmt.Printf("\n\n--- Running Bazel with all incompatible flags\n\n")
	fmt.Printf("bazel %s\n", strings.Join(args, " "))
	exitCode, err := runBazel(bazelPath, args)
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
	fmt.Printf("bazel %s\n", strings.Join(args, " "))
	exitCode, err = runBazel(bazelPath, args)
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
	for _, arg := range newArgs {
		args = insertArgs(baseArgs, []string{arg})
		fmt.Printf("\n\n--- Running Bazel with %s\n\n", arg)
		fmt.Printf("bazel %s\n", strings.Join(args, " "))
		exitCode, err = runBazel(bazelPath, args)
		if err != nil {
			log.Fatalf("could not run Bazel: %v", err)
		}
		if exitCode == 0 {
			passList = append(passList, arg)
		} else {
			failList = append(failList, arg)
		}
	}

	// 4. Print report
	fmt.Printf("\n\n+++ Result\n\n")
	fmt.Printf("Command was successful with the following flags:\n")
	for _, arg := range passList {
		fmt.Printf("  %s\n", arg)
	}
	fmt.Printf("\n")
	fmt.Printf("Migration is needed for the following flags:\n")
	for _, arg := range failList {
		fmt.Printf("  %s\n", arg)
	}

	os.Exit(1)
}

func main() {
	bazeliskHome := os.Getenv("BAZELISK_HOME")
	if len(bazeliskHome) == 0 {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			log.Fatalf("could not get the user's cache directory: %v", err)
		}

		bazeliskHome = filepath.Join(userCacheDir, "bazelisk")
	}

	err := os.MkdirAll(bazeliskHome, 0755)
	if err != nil {
		log.Fatalf("could not create directory %s: %v", bazeliskHome, err)
	}

	bazelVersion, err := getBazelVersion()
	if err != nil {
		log.Fatalf("could not get Bazel version: %v", err)
	}

	resolvedBazelVersion, isCommit, err := resolveVersionLabel(bazeliskHome, bazelVersion)
	if err != nil {
		log.Fatalf("could not resolve the version '%s' to an actual version number: %v", bazelVersion, err)
	}

	bazelDirectory := filepath.Join(bazeliskHome, "bin")
	err = os.MkdirAll(bazelDirectory, 0755)
	if err != nil {
		log.Fatalf("could not create directory %s: %v", bazelDirectory, err)
	}

	bazelPath, err := downloadBazel(resolvedBazelVersion, isCommit, bazelDirectory)
	if err != nil {
		log.Fatalf("could not download Bazel: %v", err)
	}

	args := os.Args[1:]

	// --strict and --migrate must be the first argument.
	if len(args) > 0 && (args[0] == "--strict" || args[0] == "--migrate") {
		cmd := args[0]
		newFlags, err := getIncompatibleFlags(bazeliskHome, resolvedBazelVersion)
		if err != nil {
			log.Fatalf("could not get the list of incompatible flags: %v", err)
		}

		if cmd == "--migrate" {
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

	exitCode, err := runBazel(bazelPath, args)
	if err != nil {
		log.Fatalf("could not run Bazel: %v", err)
	}
	os.Exit(exitCode)
}
