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
	"runtime"
	"sort"
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

func getBazelVersion() string {
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
		return bazelVersion
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		log.Fatalf("Could not get working directory: %v", err)
	}

	workspaceRoot := findWorkspaceRoot(workingDirectory)
	if len(workspaceRoot) != 0 {
		bazelVersionPath := path.Join(workspaceRoot, ".bazelversion")
		if _, err := os.Stat(bazelVersionPath); err == nil {
			f, err := os.Open(bazelVersionPath)
			if err != nil {
				log.Fatalf("Could not read %s: %v", bazelVersionPath, err)
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			scanner.Scan()
			bazelVersion := scanner.Text()
			if err := scanner.Err(); err != nil {
				log.Fatalf("Could not read version from file %s: %v", bazelVersion, err)
			}

			return bazelVersion
		}
	}

	return "latest"
}

type release struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
}

func resolveLatestVersion() string {
	// We could also use go-github here, but I can't get it to build with Bazel's rules_go and it pulls in a lot of dependencies.
	res, err := http.Get("https://api.github.com/repos/bazelbuild/bazel/releases")
	if err != nil {
		log.Fatalf("Could not fetch list of Bazel releases from GitHub: %v", err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("Could not read list of Bazel releases from GitHub: %v", err)
	}

	if res.StatusCode != 200 {
		log.Fatalf("Unexpected status code while reading list of Bazel releases from GitHub: %v", res.StatusCode)
	}

	var releases []release
	err = json.Unmarshal(body, &releases)
	if err != nil {
		log.Fatalf("Could not parse JSON into list of releases: %v", err)
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
	return versions[len(versions)-1].Original()
}

func resolveVersionLabel(bazeliskHome, bazelVersion string) string {
	if bazelVersion != "latest" {
		return bazelVersion
	}

	latestCache := path.Join(bazeliskHome, "latest_bazel")
	if cacheStat, err := os.Stat(latestCache); err == nil {
		if time.Since(cacheStat.ModTime()).Hours() < 1 {
			f, err := os.Open(latestCache)
			if err != nil {
				log.Fatalf("Could not read %s: %v", latestCache, err)
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			scanner.Scan()
			latestVersion := scanner.Text()
			if err := scanner.Err(); err != nil {
				log.Fatalf("Could not read latest version from file %s: %v", latestCache, err)
			}

			return latestVersion
		}
	}

	latestVersion := resolveLatestVersion()
	f, err := os.Create(latestCache)
	if err != nil {
		log.Fatalf("Could not create %s: %v", latestCache, err)
	}
	defer f.Close()
	if _, err := f.WriteString(latestVersion); err != nil {
		log.Fatalf("Could not write latest version to %s: %v", latestCache, err)
	}

	return latestVersion
}

func determineBazelFilename(version string) string {
	var machineName string
	switch runtime.GOARCH {
	case "amd64":
		machineName = "x86_64"
	default:
		log.Fatalf("Unsupported machine architecture \"%s\". Bazel currently only supports x86_64.", runtime.GOARCH)
	}

	var osName string
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		osName = runtime.GOOS
	default:
		log.Fatalf("Unsupported operating system \"%s\". Bazel currently only supports Linux, macOS and Windows.", runtime.GOOS)
	}

	filenameSuffix := ""
	if runtime.GOOS == "windows" {
		filenameSuffix = ".exe"
	}

	return fmt.Sprintf("bazel-%s-%s-%s%s", version, osName, machineName, filenameSuffix)
}

func determineURL(version, filename string) string {
	kind := "release"
	if strings.Contains(version, "rc") {
		kind = strings.SplitAfter(version, "rc")[1]
	}

	return fmt.Sprintf("https://releases.bazel.build/%s/%s/%s", version, kind, filename)
}

func downloadBazel(version, directory string) string {
	filename := determineBazelFilename(version)
	url := determineURL(version, filename)
	destinationPath := path.Join(directory, filename)

	if _, err := os.Stat(destinationPath); err != nil {
		log.Printf("Downloading %s...", url)
		f, err := os.Create(destinationPath)
		if err != nil {
			log.Fatalf("Could not create %s: %v", destinationPath, err)
		}
		defer f.Close()

		resp, err := http.Get(url)
		if err != nil {
			log.Fatalf("Could not download %s: %v", url, err)
		}
		defer resp.Body.Close()

		_, err = io.Copy(f, resp.Body)
		if err != nil {
			log.Fatalf("Could not download %s: %v", url, err)
		}
	}

	os.Chmod(destinationPath, 0755)
	return destinationPath
}

func runBazel(bazel string, args []string) int {
	cmd := exec.Command(bazel, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		log.Fatalf("Could not start Bazel: %v", err)
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
			return waitStatus.ExitStatus()
		}
		log.Fatalf("Could not launch Bazel: %v", err)
	}
	return 0
}

func main() {
	bazeliskHome := os.Getenv("BAZELISK_HOME")
	if len(bazeliskHome) == 0 {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			log.Fatalf("Could not get the user's cache directory: %v", err)
		}

		bazeliskHome = path.Join(userCacheDir, "bazelisk")
	}

	err := os.MkdirAll(bazeliskHome, 0755)
	if err != nil {
		log.Fatalf("Could not create directory %s: %v", bazeliskHome, err)
	}

	bazelVersion := getBazelVersion()
	resolvedBazelVersion := resolveVersionLabel(bazeliskHome, bazelVersion)
	if err != nil {
		log.Fatalf("Could not resolve the version '%s' to an actual version number: %v", bazelVersion, err)
	}

	bazelDirectory := path.Join(bazeliskHome, "bin")
	err = os.MkdirAll(bazelDirectory, 0755)
	if err != nil {
		log.Fatalf("Could not create directory %s: %v", bazelDirectory, err)
	}

	bazelPath := downloadBazel(resolvedBazelVersion, bazelDirectory)
	if err != nil {
		log.Fatalf("Could not download Bazel: %v", err)
	}

	exitCode := runBazel(bazelPath, os.Args[1:])
	os.Exit(exitCode)
}
