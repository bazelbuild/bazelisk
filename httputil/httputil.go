// Package httputil offers functions to read and download files via HTTP.
package httputil

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var (
	// DefaultTransport specifies the http.RoundTripper that is used for any network traffic, and may be replaced with a dummy implementation for unit testing.
	DefaultTransport = http.DefaultTransport
)

func getClient() *http.Client {
	return &http.Client{Transport: DefaultTransport}
}

// ReadRemoteFile returns the contents of the given file, using the supplied Authorization token, if set.
func ReadRemoteFile(url string, token string) ([]byte, error) {
	client := getClient()
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
		resp, err := getClient().Get(originURL)
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

// MaybeDownload downloads a file from the given url and caches the result under bazeliskHome.
// It skips the download if the file already exists and is not outdated.
// Parameter ´description´ is only used to provide better error messages.
func MaybeDownload(bazeliskHome, url, filename, description, token string) ([]byte, error) {
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
	body, err := ReadRemoteFile(url, token)
	if err != nil {
		return nil, fmt.Errorf("could not download %s: %v", description, err)
	}

	err = ioutil.WriteFile(cachePath, body, 0666)
	if err != nil {
		return nil, fmt.Errorf("could not create %s: %v", cachePath, err)
	}

	return body, nil
}
