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
emVsLWRldkBnb29nbGVncm91cHMuY29tPokCPgQTAQIAKAIbAwYLCQgHAwIGFQgC
CQoLBBYCAwECHgECF4AFAlsGueoFCQeEhaQACgkQPVkZtEhFfuCojRAAqtUaEbK8
zVAPssZDRPun0k1XB3hXxEoe5kt00cl51F+KLXN2OM5gOn2PcUw4A+Ci+48cgt9b
hTWwWuC9OPn9OCvYVyuTJXT189Pmg+F9l3zD/vrD5gdFKDLJCUPo/tRBTDQqrRGA
JssWIzvGR65O2AosoIcj7VAfNj34CBHm25abNpGnWmkiREZzElLFqjTR+FwAMxyA
VJnPbn+K1zyi9xUZKcL1QzKcHBTPFAdZR6zTII/+03n4wAL/w8+x/A1ocmE7jxCI
cgq7vaHSpGmigU2+TXckUslIgIC64iqYBpPvFAPNlqXmo9rDfL2Imyyuz1ep7j/b
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
MZaF+SBEZXd1ZSqB5Q==
=laPs
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
