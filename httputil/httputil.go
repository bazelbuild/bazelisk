// Package httputil offers functions to read and download files via HTTP.
package httputil

import (
	b64 "encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/ProtonMail/gopenpgp/v3/crypto"

	netrc "github.com/bgentry/go-netrc/netrc"
	homedir "github.com/mitchellh/go-homedir"

	"github.com/bazelbuild/bazelisk/config"
	"github.com/bazelbuild/bazelisk/httputil/progress"
)

var (
	// DefaultTransport specifies the http.RoundTripper that is used for any network traffic, and may be replaced with a dummy implementation for unit testing.
	DefaultTransport = http.DefaultTransport
	// UserAgent is passed to every HTTP request as part of the 'User-Agent' header.
	UserAgent   = "Bazelisk"
	linkPattern = regexp.MustCompile(`<(.*?)>; rel="(\w+)"`)

	// RetryClock is used for waiting between HTTP request retries.
	RetryClock = Clock(&realClock{})

	// VerificationKey is the public PGP key used to verify Bazel binary signatures.
	VerificationKey = `
-----BEGIN PGP PUBLIC KEY BLOCK-----

mQINBFdEmzkBEACzj8tMYUau9oFZWNDytcQWazEO6LrTTtdQ98d3JcnVyrpT16yg
I/QfGXA8LuDdKYpUDNjehLtBL3IZp4xe375Jh8v2IA2iQ5RXGN+lgKJ6rNwm15Kr
qYeCZlU9uQVpZuhKLXsWK6PleyQHjslNUN/HtykIlmMz4Nnl3orT7lMI5rsGCmk0
1Kth0DFh8SD9Vn2G4huddwxM8/tYj1QmWPCTgybATNuZ0L60INH8v6+J2jJzViVc
NRnR7mpouGmRy/rcr6eY9QieOwDou116TrVRFfcBRhocCI5b6uCRuhaqZ6Qs28Bx
4t5JVksXJ7fJoTy2B2s/rPx/8j4MDVEdU8b686ZDHbKYjaYBYEfBqePXScp8ndul
XWwS2lcedPihOUl6oQQYy59inWIpxi0agm0MXJAF1Bc3ToSQdHw/p0Y21kYxE2pg
EaUeElVccec5poAaHSPprUeej9bD9oIC4sMCsLs7eCQx2iP+cR7CItz6GQtuZrvS
PnKju1SKl5iwzfDQGpi6u6UAMFmc53EaH05naYDAigCueZ+/2rIaY358bECK6/VR
kyrBqpeq6VkWUeOkt03VqoPzrw4gEzRvfRtLj+D2j/pZCH3vyMYHzbaaXBv6AT0e
RmgtGo9I9BYqKSWlGEF0D+CQ3uZfOyovvrbYqNaHynFBtrx/ZkM82gMA5QARAQAB
tEdCYXplbCBEZXZlbG9wZXIgKEJhemVsIEFQVCByZXBvc2l0b3J5IGtleSkgPGJh
emVsLWRldkBnb29nbGVncm91cHMuY29tPokCVQQTAQgAPwIbAwYLCQgHAwIGFQgC
CQoLBBYCAwECHgECF4AWIQRxodDvz+tigf0EN8k9WRm0SEV+4AUCXsoWGgUJC0fh
4QAKCRA9WRm0SEV+4NDCD/9c5rhZREBlikdi5QYRq1YOkwzJLXFoVe0FonEwMuWK
fQzT/rIwyh14tssptU5+eXwTEXL0ZDskgzvrFSpzjQZzcSG/gzNCATNfrZpC2nfE
SxMKOeIwQedn26YIHCI8s9tEQ7BSvfBfJgqfIo3IURhmfzNMj+qszca+3IDYAlAy
8lxUVbJcIQ0apnAdnIadtydzca56mMN7ma+btddaWLpAdyfUvQ/Zsx3TYYLF7inQ
km0JpzISN0fGngzGNDGNmtHNhCdSpyfkr+7fvpbKAYkSH7uZ1AIPDyHdLIwDQnX2
kbLRkxKncKGSDhUSdlJTl0x36cU+xmgO15FFdOyk3BUfrlfDrgXIBjeX8KNh9TV6
HgFFR/mNONoJ93ZvZQNO2s1gbPZJe3VJ1Q5PMLW1sdl8q8JthBwT/5TJ1k8E5VYj
jAc8dl+RAALxqj+eo5xI45o1FdV5s1aGDjbwFoCIhGCy2zaog1q5wnhmEptAAD0S
TVbJSpwNiLlPIcGVaCjXp8Ow3SzOGTRKIjFTO/I6FiSJOpgfri07clXmnb4ETjou
mUdglg8/8nQ120zHEOqoSzzIbTNUDjNZY8SuY6Ig3/ObQ/JAFS0i6h74KLfXUZzn
uETY7KURLdyPAhL37Hb9FDhvkJCUO/l6eqDh9jk1JjB7Cvb7hEvnbvDrr2hWNAL7
RrkCDQRXRJs5ARAA55/1VBlDpV/ElUyLmRyPCz/V+msHdinyw4Mv5DJQupuZwlMy
vxPPzc7GmsIfk1zuOzDWirNs22r43ak6dsAvpcU+iVBi46MqUcbNtC+kfxlKiToD
PCs82rdfCgHT7XYDzrCWlqNQ9++BqM2OYRIxyEucizeofWPlrJUgKvu8fWLVZ6bY
n4L/PqAhobhuSjRcoB5Tp81hGa4cscKIGIqhymfnguaY8viJ83tHPUqQJoApNPy8
q1pWHSDV6zBv71beqV2b6cBzp7VqNYOIuqE6ZNBFWuCG3zRc9ia2/bHxx2TGAQJt
PpPzitm0xkB3GGN06YnnSCE+f2j+7F0IO6uFlSy7ho0PoSFbDgR91kJK3S0ZBZx4
H21cIpWWBzf9Nd1M4H3O7KhnGSZDq6+tXZ9/F/ZUvCZHpQlJewDPY9315Ymacf5C
Zk8xeE5UUIxFMdOxF8B7Itb6rbFWv+tzWdX/0/M8/b0ZJhVvngWzuh/agdS4E5an
f7ahGWM96jPRIQEb9DRN2YGp9hOiX2sZqkhxE5zWqD2gdXp2ZAxMCTHf4ijzOVsO
nde7b5BqC0JL73gNwf1iOHyCAzqGiFfah8/odBTDhMsdVMsjSIxzcwlwRnzy+hBs
dYpP19ieJCMoERJTbUgSspPdhY/Y4ChzlFHjiAKYT6vXiYcKS04stCtHqwEAEQEA
AYkCPAQYAQgAJgIbDBYhBHGh0O/P62KB/QQ3yT1ZGbRIRX7gBQJeyhYlBQkLR+Hs
AAoJED1ZGbRIRX7g3Y8P/iuOAHmyCMeSELvUs9ZvLYJKGzmz67R8fJSmgst/Bs3p
dWCAjGE56M6UgZzHXK+fBRWFPDOXT64XNq0UIG7tThthwe4Gdvg/5rWG61Pe/vCZ
2FkMAlEMkuufZYMcw9jItHMKLcYyW/jtN9EzCX+vM6SZlu4o8la5rCIBEaiKfzft
a/dRMjW+RqQnU31NQCDAy3zoGUCQumJtv3GVbMYHIrRZua2yyNo9Iborh2SVdBbK
v9WJKH4JcCHd0/XDGdys6EXeATIIRxchumkmxpIg87OhsC0n5yuH1FnFIFQEjbYX
bb46F7ZFT+8Tov+lgMEw4CZmps4uvvZlKbIH4Zi/ULiobwvm2ad3nejWICmGmHYz
ro6t08hdcY6GnOzCpDwx9yHechMCkU3KEE98nb/CxcmA4VzDHudTJe7o0OyaSarh
6D5WcXf7D9FfcKmUD9xaCsfXh66OCksMVGE1JctrO1wQTF2jTdTUq7mmi30tlM+o
JjVk65OSOd4JYol8auzE4oXOfsNzXbyvj7WzM1v5m7C45jOL+Ly7I3IUzZNfF41J
AMmSd73EOoR9YH4qTrL3jx69Ekf7ww70Qea5enLE8xUgQfGTOaEHxkFcEovmzv54
6IVe083iK8alXD/9OUTaDY9NwMnOn1K1aU2XOfliGGLgwwaHg+wVFh5rZIHsDl7v
=Embu
-----END PGP PUBLIC KEY BLOCK-----
`

	// MaxRetries specifies how often non-fatally failing HTTP requests should be retried.
	MaxRetries = 4
	// MaxRequestDuration defines the maximum amount of time that a request and its retries may take in total
	MaxRequestDuration = time.Second * 30
	retryHeaders       = []string{"Retry-After", "X-RateLimit-Reset", "Rate-Limit-Reset"}
	NotFound           = errors.New("not found")
)

// Clock keeps track of time. It can return the current time, as well as move forward by sleeping for a certain period.
type Clock interface {
	Sleep(time.Duration)
	Now() time.Time
}

type realClock struct{}

func (*realClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (*realClock) Now() time.Time {
	return time.Now()
}

// ReadRemoteFile returns the contents of the given file, using the supplied Authorization header value, if set. It also returns the HTTP headers.
// If the request fails with a transient error it will retry the request for at most MaxRetries times.
// It obeys HTTP headers such as "Retry-After" when calculating the start time of the next attempt.
// If no such header is present, it uses an exponential backoff strategy.
func ReadRemoteFile(url string, auth string) ([]byte, http.Header, error) {
	res, err := get(url, auth)
	if err != nil {
		return nil, nil, fmt.Errorf("could not fetch %s: %w", url, err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, res.Header, fmt.Errorf("unexpected status code while reading %s: %v", url, res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, res.Header, fmt.Errorf("failed to read content at %s: %v", url, err)
	}
	return body, res.Header, nil
}

func get(url, auth string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create request: %v", err)
	}

	req.Header.Set("User-Agent", UserAgent)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	client := &http.Client{Transport: DefaultTransport}
	deadline := RetryClock.Now().Add(MaxRequestDuration)
	var lastFailure string
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		res, err := client.Do(req)
		if !shouldRetry(res, err) {
			return res, err
		}

		if res != nil {
			// Need to retry, close the response body immediately to release resources.
			// See https://github.com/googleapis/google-cloud-go/issues/7440#issuecomment-1491008639
			res.Body.Close()
		}

		if err == nil {
			lastFailure = fmt.Sprintf("HTTP %d", res.StatusCode)
		} else {
			lastFailure = err.Error()
		}
		waitFor, err := getWaitPeriod(res, err, attempt)
		if err != nil {
			return nil, err
		}

		nextTryAt := RetryClock.Now().Add(waitFor)
		if nextTryAt.After(deadline) {
			return nil, fmt.Errorf("unable to complete %d requests to %s within %v. Most recent failure: %s", attempt+1, url, MaxRequestDuration, lastFailure)
		}
		if attempt < MaxRetries {
			RetryClock.Sleep(waitFor)
		}
	}
	return nil, fmt.Errorf("unable to complete request to %s after %d retries. Most recent failure: %s", url, MaxRetries, lastFailure)
}

func shouldRetry(res *http.Response, err error) bool {
	// Retry if the client failed to speak HTTP.
	if err != nil {
		return true
	}
	// For HTTP: only retry on non-permanent/fatal errors.
	return res.StatusCode == 429 || (500 <= res.StatusCode && res.StatusCode <= 504)
}

func getWaitPeriod(res *http.Response, err error, attempt int) (time.Duration, error) {
	if err == nil {
		// If HTTP works, check if the server told us when to retry
		for _, header := range retryHeaders {
			if value := res.Header[header]; len(value) > 0 {
				return parseRetryHeader(value[0])
			}
		}
	}
	// Let's just use exponential backoff: 1s + d1, 2s + d2, 4s + d3, 8s + d4 with dx being a random value in [0ms, 500ms]
	return time.Duration(1<<uint(attempt))*time.Second + time.Duration(rand.Intn(500))*time.Millisecond, nil
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

// tryFindNetrcFileCreds returns base64-encoded login:password found in ~/.netrc file for a given `host`
func tryFindNetrcFileCreds(host string) (string, error) {
	dir, err := homedir.Dir()
	if err != nil {
		return "", err
	}

	var file = filepath.Join(dir, ".netrc")
	n, err := netrc.ParseFile(file)
	if err != nil {
		// netrc does not exist or we can't read it
		return "", err
	}

	m := n.FindMachine(host)
	if m == nil {
		// if host is not found, we should proceed without providing any Authorization header,
		// because remote host may not have auth at all.
		log.Printf("Skipping basic authentication for %s because no credentials found in %s", host, file)
		return "", fmt.Errorf("could not find creds for %s in netrc %s", host, file)
	}

	log.Printf("Using basic authentication credentials for host %s from %s", host, file)

	token := b64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", m.Login, m.Password)))
	return fmt.Sprintf("Basic %s", token), nil
}

func getAuthForURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		// rawURL is supposed to be valid
		return "", err
	}

	t, err := tryFindNetrcFileCreds(u.Host)
	if err != nil {
		return "", nil
	}
	return t, nil
}

type DownloadArtifact struct {
	BinaryPath    string
	SignaturePath string
}

func DownloadFile(url string, destFile *os.File, config config.Config) error {
	auth, err := getAuthForURL(url)
	if err != nil {
		return err
	}

	log.Printf("Downloading %s...", url)
	resp, err := get(url, auth)
	if err != nil {
		return fmt.Errorf("HTTP GET %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return NotFound
	} else if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP GET %s failed with error %v", url, resp.StatusCode)
	}

	_, err = io.Copy(
		// Add a progress bar during download.
		progress.Writer(destFile, "Downloading", resp.ContentLength, config),
		resp.Body)
	progress.Finish(config)
	if err != nil {
		return fmt.Errorf("could not copy from %s to %s: %v", url, destFile.Name(), err)
	}

	return nil
}

func createTempFile(destDir, pattern string) (*os.File, func(), error) {
	tmpFile, err := os.CreateTemp(destDir, pattern)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create temporary file: %v", err)
	}
	return tmpFile, func() {
		err := tmpFile.Close()
		if err == nil {
			os.Remove(tmpFile.Name())
		}
	}, nil
}

func VerifyBinary(binary, signature io.Reader, verificationKey string) (*crypto.VerifyResult, error) {
	pgp := crypto.PGP()
	key, err := crypto.NewKeyFromArmored(verificationKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load the embedded Verification Key: %v", err)
	}

	keys, err := crypto.NewKeyRing(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %v", err)
	}

	verifier, err := pgp.Verify().
		VerificationKeys(keys).
		New()
	if err != nil {
		return nil, fmt.Errorf("failed to create verifier: %v", err)
	}

	verifyDataReader, err := verifier.VerifyingReader(binary, signature, crypto.Auto)
	if err != nil {
		return nil, fmt.Errorf("failed to create verifying reader: %v", err)
	}

	result, err := verifyDataReader.DiscardAllAndVerifySignature()
	if err != nil {
		return nil, fmt.Errorf("failed to verify authenticity of downloaded file: %v", err)
	}

	return result, nil
}

// DownloadBinary downloads a file from the given URL into the specified location, marks it executable and returns its full path.
func DownloadBinary(originURL, signatureURL, destDir, destFile string, config config.Config) (DownloadArtifact, error) {
	err := os.MkdirAll(destDir, 0755)
	if err != nil {
		return DownloadArtifact{}, fmt.Errorf("could not create directory %s: %v", destDir, err)
	}
	destinationPath := filepath.Join(destDir, destFile)
	destinationSignaturePath := destinationPath + ".sig"

	if signatureURL == "" && config.Get("BAZELISK_NO_SIGNATURE_VERIFICATION") == "" {
		return DownloadArtifact{}, fmt.Errorf("signature verification is requested, but no signature URL was provided")
	}

	if _, err := os.Stat(destinationPath); err != nil {
		originTmpFile, originCleanFunc, err := createTempFile(destDir, "download")
		if err != nil {
			return DownloadArtifact{}, fmt.Errorf("could not create temporary file: %v", err)
		}
		defer originCleanFunc()

		err = DownloadFile(originURL, originTmpFile, config)
		if err != nil {
			return DownloadArtifact{}, fmt.Errorf("failed to download %s: %v", originURL, err)
		}
		err = os.Chmod(originTmpFile.Name(), 0755)
		if err != nil {
			return DownloadArtifact{}, fmt.Errorf("could not chmod file %s: %v", originTmpFile.Name(), err)
		}

		// download the signature file if signature verification is requested
		if config.Get("BAZELISK_NO_SIGNATURE_VERIFICATION") == "" && signatureURL != "" {
			signatureTmpFile, signatureCleanFunc, err := createTempFile(destDir, "download-signature-")
			if err != nil {
				return DownloadArtifact{}, fmt.Errorf("could not create temporary file: %v", err)
			}
			defer signatureCleanFunc()

			err = DownloadFile(signatureURL, signatureTmpFile, config)
			if err != nil {
				return DownloadArtifact{}, fmt.Errorf("failed to download %s: %v", signatureURL, err)
			}

			signatureTmpFile.Close()
			err = os.Rename(signatureTmpFile.Name(), destinationSignaturePath)
			if err != nil {
				return DownloadArtifact{}, fmt.Errorf("could not move %s to %s: %v", signatureTmpFile.Name(), destinationSignaturePath, err)
			}
		}

		originTmpFile.Close()
		err = os.Rename(originTmpFile.Name(), destinationPath)
		if err != nil {
			return DownloadArtifact{}, fmt.Errorf("could not move %s to %s: %v", originTmpFile.Name(), destinationPath, err)
		}
	} else if config.Get("BAZELISK_NO_SIGNATURE_VERIFICATION") == "" {
		if _, err := os.Stat(destinationSignaturePath); err != nil {
			return DownloadArtifact{}, fmt.Errorf("%s already exists, but corresponding signature file %s does not exist or unaccessable: %v", destinationPath, destinationSignaturePath, err)
		}
	}

	return DownloadArtifact{destinationPath, destinationSignaturePath}, nil
}

// ContentMerger is a function that merges multiple HTTP payloads into a single message.
type ContentMerger func([][]byte) ([]byte, error)

// MaybeDownload downloads a file from the given url and caches the result under bazeliskHome.
// It skips the download if the file already exists and is not outdated.
// Parameter ´description´ is only used to provide better error messages.
// Parameter `auth` is a value of "Authorization" HTTP header.
func MaybeDownload(bazeliskHome, url, filename, description, auth string, merger ContentMerger) ([]byte, error) {
	cachePath := filepath.Join(bazeliskHome, filename)
	if cacheStat, err := os.Stat(cachePath); err == nil {
		if time.Since(cacheStat.ModTime()).Hours() < 1 {
			res, err := os.ReadFile(cachePath)
			if err != nil {
				return nil, fmt.Errorf("could not read %s: %v", cachePath, err)
			}
			return res, nil
		}
	}

	contents := make([][]byte, 0)
	nextURL := url
	for nextURL != "" {
		// We could also use go-github here, but I can't get it to build with Bazel's rules_go and it pulls in a lot of dependencies.
		body, headers, err := ReadRemoteFile(nextURL, auth)
		if err != nil {
			return nil, fmt.Errorf("could not download %s: %v", description, err)
		}
		contents = append(contents, body)
		nextURL = getNextURL(headers)
	}

	merged, err := merger(contents)
	if err != nil {
		return nil, fmt.Errorf("failed to merge %d chunks from %s: %v", len(contents), url, err)
	}

	err = os.WriteFile(cachePath, merged, 0666)
	if err != nil {
		return nil, fmt.Errorf("could not create %s: %v", cachePath, err)
	}

	return merged, nil
}

func getNextURL(headers http.Header) string {
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
