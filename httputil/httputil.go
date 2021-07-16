// Package httputil offers functions to read and download files via HTTP.
package httputil

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

var (
	// DefaultTransport specifies the http.RoundTripper that is used for any network traffic, and may be replaced with a dummy implementation for unit testing.
	DefaultTransport = http.DefaultTransport
	UserAgent = "Bazelisk"
	linkPattern = regexp.MustCompile(`<(.*?)>; rel="(\w+)"`)

	RetryClock = Clock(&realClock{})
	MaxRetries = 4
	// MaxRequestDuration defines the maximum amount of time that a request and its retries may take in total
	MaxRequestDuration = time.Second * 10
	retryHeaders = []string{"Retry-After", "X-RateLimit-Reset", "Rate-Limit-Reset"}
)

type Clock interface {
	Sleep(time.Duration)
	Now() time.Time
}

type realClock struct {}

func (*realClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (*realClock) Now() time.Time {
	return time.Now()
}

// ReadRemoteFile returns the contents of the given file, using the supplied Authorization token, if set. It also returns the HTTP headers.
// If the request fails with a transient error it will retry the request for at most MaxRetries times.
// It obeys HTTP headers such as "Retry-After" when calculating the start time of the next attempt.
// If no such header is present, it uses an exponential backoff strategy.
func ReadRemoteFile(url string, token string) ([]byte, http.Header, error) {
	res, err := get(url, token)
	if err != nil {
		return nil, nil, fmt.Errorf("could not fetch %s: %v", url, err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, res.Header, fmt.Errorf("unexpected status code while reading %s: %v", url, res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, res.Header, fmt.Errorf("failed to read content at %s: %v", url, err)
	}
	return body, res.Header, nil
}

func get(url, token string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create request: %v", err)
	}

	req.Header.Set("User-Agent", UserAgent)
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	client := &http.Client{Transport: DefaultTransport}
	deadline := RetryClock.Now().Add(MaxRequestDuration)
	lastStatus := 0
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		res, err := client.Do(req)
		// Do not retry on success and permanent/fatal errors
		if err != nil || !shouldRetry(res) {
			return res, err
		}

		lastStatus = res.StatusCode
		waitFor, err := getWaitPeriod(res, attempt)
		if err != nil {
			return nil, err
		}

		nextTryAt := RetryClock.Now().Add(waitFor)
		if nextTryAt.After(deadline) {
			return nil, fmt.Errorf("unable to complete request to %s within %v", url, MaxRequestDuration)
		}
		if attempt < MaxRetries {
			RetryClock.Sleep(waitFor)
		}
	}
	return nil, fmt.Errorf("unable to complete request to %s after %d retries. Most recent status: %d", url, MaxRetries, lastStatus)
}

func shouldRetry(res *http.Response) bool {
	return res.StatusCode == 429 || (500 <= res.StatusCode && res.StatusCode <= 504)
}

func getWaitPeriod(res *http.Response, attempt int) (time.Duration, error) {
	// Check if the server told us when to retry
	for _, header := range retryHeaders {
		if value := res.Header[header]; len(value) > 0 {
			return parseRetryHeader(value[0])
		}
	}
	// Let's just use exponential backoff: 1s + d1, 2s + d2, 4s + d3, 8s + d4 with dx being a random value in [0ms, 500ms]
	return time.Duration(1 << attempt) * time.Second + time.Duration(rand.Intn(500)) * time.Millisecond, nil
}

func parseRetryHeader(value string) (time.Duration, error) {
	// Depending on the server the header value can be a number of seconds (how long to wait) or an actual date (when to retry).
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Second * time.Duration(seconds), nil
	}
	t, err := http.ParseTime(value)
	if err != nil {
		return 0, err
	}
	return time.Until(t), nil
}

// DownloadBinary downloads a file from the given URL into the specified location, marks it executable and returns its full path.
func DownloadBinary(originURL, destDir, destFile string) (string, error) {
	err := os.MkdirAll(destDir, 0755)
	if err != nil {
		return "", fmt.Errorf("could not create directory %s: %v", destDir, err)
	}
	destinationPath := filepath.Join(destDir, destFile)

	if _, err := os.Stat(destinationPath); err != nil {
		tmpfile, err := ioutil.TempFile(destDir, "download")
		if err != nil {
			return "", fmt.Errorf("could not create temporary file: %v", err)
		}
		defer func() {
			err := tmpfile.Close()
			if err == nil {
				os.Remove(tmpfile.Name())
			}
		}()

		log.Printf("Downloading %s...", originURL)
		resp, err := get(originURL, "")
		if err != nil {
			return "", fmt.Errorf("HTTP GET %s failed: %v", originURL, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return "", fmt.Errorf("HTTP GET %s failed with error %v", originURL, resp.StatusCode)
		}

		_, err = io.Copy(tmpfile, resp.Body)
		if err != nil {
			return "", fmt.Errorf("could not copy from %s to %s: %v", originURL, tmpfile.Name(), err)
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

type ContentMerger func([][]byte) ([]byte, error)

// MaybeDownload downloads a file from the given url and caches the result under bazeliskHome.
// It skips the download if the file already exists and is not outdated.
// Parameter ´description´ is only used to provide better error messages.
func MaybeDownload(bazeliskHome, url, filename, description, token string, merger ContentMerger) ([]byte, error) {
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

	contents := make([][]byte, 0)
	nextUrl := url
	for nextUrl != "" {
		// We could also use go-github here, but I can't get it to build with Bazel's rules_go and it pulls in a lot of dependencies.
		body, headers, err := ReadRemoteFile(nextUrl, token)
		if err != nil {
			return nil, fmt.Errorf("could not download %s: %v", description, err)
		}
		contents = append(contents, body)
		nextUrl = getNextUrl(headers)
	}

	merged, err := merger(contents)
	if err != nil {
		return nil, fmt.Errorf("failed to merge %d chunks from %s: %v", len(contents), url, err)
	}

	err = ioutil.WriteFile(cachePath, merged, 0666)
	if err != nil {
		return nil, fmt.Errorf("could not create %s: %v", cachePath, err)
	}

	return merged, nil
}

func getNextUrl(headers http.Header) string {
	links := headers["Link"]
	if len(links) != 1 {
		return ""
	}
	for _, m := range linkPattern.FindAllStringSubmatch(links[0], -1) {
		if m[2] == "next" {
			return m[1]
		}
	}	
	return ""
}
