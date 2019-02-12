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
	"path"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	version "github.com/hashicorp/go-version"
)

func findWorkspaceRoot(root string) string {
	if _, err := os.Stat(path.Join(root, "WORKSPACE")); err == nil {
		return root
	}

	parentDirectory := path.Dir(root)
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
		bazelVersionPath := path.Join(workspaceRoot, ".bazelversion")
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

// maybeDownload will download a file from the given url and cache the result under bazeliskHome.
// It skips the download if the file already exists and is not outdated.
// description is used only to provide better error messages.
func maybeDownload(bazeliskHome, url, filename, description string) ([]byte, error) {
	cachePath := path.Join(bazeliskHome, filename)

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
	res, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("could not fetch %s: %v", description, err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %v", description, err)
	}

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code while reading %s: %v", description, res.StatusCode)
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

	var versions []*version.Version
	for _, release := range releases {
		if release.Prerelease {
			continue
		}
		v, err := version.NewVersion(release.TagName)
		if err != nil {
			log.Printf("WARN: Could not parse version: %s", release.TagName)
		}
		versions = append(versions, v)
	}
	sort.Sort(version.Collection(versions))
	if offset >= len(versions) {
		return "", fmt.Errorf("cannot resolve version \"latest-%d\": There are only %d Bazel releases", offset, len(versions))
	}
	return versions[len(versions)-1-offset].Original(), nil
}

func resolveVersionLabel(bazeliskHome, bazelVersion string) (string, error) {
	r := regexp.MustCompile(`^latest(?:-(?P<offset>\d+))?$`)

	match := r.FindStringSubmatch(bazelVersion)
	if match != nil {
		offset := 0
		if match[1] != "" {
			var err error
			offset, err = strconv.Atoi(match[1])
			if err != nil {
				return "", fmt.Errorf("invalid version \"%s\", could not parse offset: %v", bazelVersion, err)
			}
		}
		return resolveLatestVersion(bazeliskHome, offset)
	}

	return bazelVersion, nil
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

func determineURL(version, filename string) string {
	kind := "release"
	if strings.Contains(version, "rc") {
		kind = strings.SplitAfter(version, "rc")[1]
	}

	return fmt.Sprintf("https://releases.bazel.build/%s/%s/%s", version, kind, filename)
}

func downloadBazel(version, directory string) (string, error) {
	filename, err := determineBazelFilename(version)
	if err != nil {
		return "", fmt.Errorf("could not determine filename to use for Bazel binary: %v", err)
	}

	url := determineURL(version, filename)
	destinationPath := path.Join(directory, filename)

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

func runBazel(bazel string, args []string) (int, error) {
	cmd := exec.Command(bazel, args...)
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
	url := "https://api.github.com/search/issues?q=repo:bazelbuild/bazel+label:migration-" + version
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

	return result, nil
}

func migrate(bazelPath string, baseArgs []string, newArgs []string) {
	// 1. Try with all the flags.
	args := append(baseArgs, newArgs...)
	fmt.Printf("Running Bazel with %q\n", args)
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
	fmt.Printf("\n---\n\n")
	fmt.Printf("Running Bazel with %q\n", args)
	exitCode, err = runBazel(bazelPath, args)
	if err != nil {
		log.Fatalf("could not run Bazel: %v", err)
	}
	if exitCode != 0 {
		fmt.Printf("Failure: Command failed, even without incomaptible flags.\n")
		os.Exit(0)
	}

	// 3. Try with each flag separately.
	var passList []string
	var failList []string
	for _, arg := range newArgs {
		args = append(baseArgs, arg)
		fmt.Printf("\n---\n\n")
		fmt.Printf("Running Bazel with %q\n", args)
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
	fmt.Printf("\n---\n\n")
	fmt.Printf("Command was successful with the following flags:\n")
	for _, arg := range passList {
		fmt.Printf("  %s\n", arg)
	}
	fmt.Printf("\n")
	fmt.Printf("Migration is needed for the following flags:\n")
	for _, arg := range failList {
		fmt.Printf("  %s\n", arg)
	}

	os.Exit(0)
}

func main() {
	bazeliskHome := os.Getenv("BAZELISK_HOME")
	if len(bazeliskHome) == 0 {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			log.Fatalf("could not get the user's cache directory: %v", err)
		}

		bazeliskHome = path.Join(userCacheDir, "bazelisk")
	}

	err := os.MkdirAll(bazeliskHome, 0755)
	if err != nil {
		log.Fatalf("could not create directory %s: %v", bazeliskHome, err)
	}

	bazelVersion, err := getBazelVersion()
	if err != nil {
		log.Fatalf("could not get Bazel version: %v", err)
	}

	resolvedBazelVersion, err := resolveVersionLabel(bazeliskHome, bazelVersion)
	if err != nil {
		log.Fatalf("could not resolve the version '%s' to an actual version number: %v", bazelVersion, err)
	}

	bazelDirectory := path.Join(bazeliskHome, "bin")
	err = os.MkdirAll(bazelDirectory, 0755)
	if err != nil {
		log.Fatalf("could not create directory %s: %v", bazelDirectory, err)
	}

	bazelPath, err := downloadBazel(resolvedBazelVersion, bazelDirectory)
	if err != nil {
		log.Fatalf("could not download Bazel: %v", err)
	}

	args := os.Args[1:]

	// --strict must be the first argument. When it is present, it expands to the list of
	// --incompatible_ flags that should be enabled for the given Bazel version.
	if len(args) > 0 && args[0] == "--strict" || args[0] == "--migrate" {
		cmd := args[0]
		newFlags, err := getIncompatibleFlags(bazeliskHome, resolvedBazelVersion)
		if err != nil {
			log.Fatalf("could not get the list of incompatible flags: %v", err)
		}

		if cmd == "--migrate" {
			migrate(bazelPath, args[1:], newFlags)
		} else {
			args = append(args[1:], newFlags...)
		}
	}

	exitCode, err := runBazel(bazelPath, args)
	if err != nil {
		log.Fatalf("could not run Bazel: %v", err)
	}
	os.Exit(exitCode)
}
