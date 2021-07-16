package httputil

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

var (
	seed = time.Now()
)

type fakeClock struct {
	now          time.Time
	SleepPeriods []time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: seed}
}

func (fc *fakeClock) Sleep(d time.Duration) {
	fc.now = fc.now.Add(d)
	fc.SleepPeriods = append(fc.SleepPeriods, d)
}

func (fc *fakeClock) Now() time.Time {
	return fc.now
}

func (fc *fakeClock) TimesSlept() int {
	return len(fc.SleepPeriods)
}

func setUp() (*FakeTransport, *fakeClock) {
	transport := NewFakeTransport()
	DefaultTransport = transport

	clock := newFakeClock()
	RetryClock = clock
	return transport, clock
}

func setUpAllFailures(url string, status, retries int, headers map[string]string) (*FakeTransport, *fakeClock) {
	MaxRetries = retries

	transport, clock := setUp()
	for i := 0; i <= retries; i++ {
		transport.AddResponse(url, status, "", headers)
	}
	return transport, clock
}

func TestSuccessOnFirstTry(t *testing.T) {
	transport, _ := setUp()

	url := "http://foo"
	want := "the_body"
	transport.AddResponse(url, 200, want, nil)
	body, _, err := ReadRemoteFile(url, "")

	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	got := string(body)
	if got != want {
		t.Fatalf("Expected body %q, but got %q", want, got)
	}
}

func TestSuccessOnRetry(t *testing.T) {
	transport, clock := setUp()

	url := "http://foo"
	want := "the_body"
	transport.AddResponse(url, 503, "", nil)
	transport.AddResponse(url, 200, want, nil)
	body, _, err := ReadRemoteFile(url, "")

	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	got := string(body)
	if got != want {
		t.Fatalf("Expected body %q, but got %q", want, got)
	}

	if clock.TimesSlept() != 1 {
		t.Fatalf("Expected a single retry, not %d", clock.TimesSlept())
	}
}

func TestAllTriesFail(t *testing.T) {
	MaxRequestDuration = 100 * time.Second

	url := "http://bar"
	retries := 5
	_, clock := setUpAllFailures(url, 502, retries, nil)

	_, _, err := ReadRemoteFile(url, "")
	if err == nil {
		t.Fatal("Expected request to fail with code 502")
	}

	reason := err.Error()
	expected := "could not fetch http://bar: unable to complete request to http://bar after 5 retries. Most recent status: 502"
	if reason != expected {
		t.Fatalf("Expected request to fail with %q, but got %q", expected, reason)
	}

	if clock.TimesSlept() != retries {
		t.Fatalf("Expected %d retries, but got %d", retries, clock.TimesSlept())
	}
}

func TestShouldObeyRetryHeaders(t *testing.T) {
	MaxRequestDuration = time.Hour * 24

	headerGens := []func(time.Duration) string{
		func(d time.Duration) string {
			// Value == earliest date to retry
			return seed.Add(d).UTC().Format(http.TimeFormat)
		},
		func(d time.Duration) string {
			// Value == seconds to wait
			return strconv.Itoa(int(d.Seconds()))
		},
	}

	for _, header := range retryHeaders {
		for _, gen := range headerGens {
			url := "http://bar"
			wanted := 5 * time.Hour
			_, clock := setUpAllFailures(url, 501, 1, map[string]string{header: gen(wanted)})

			_, _, err := ReadRemoteFile(url, "")
			if err == nil {
				t.Fatal("Expected request to fail with code 502")
			}

			if clock.TimesSlept() != 1 {
				t.Fatalf("Expected a single retry, but got %d", clock.TimesSlept())
			}

			got := clock.SleepPeriods[0]
			delta := time.Minute
			if got < wanted-delta || wanted+delta < got {
				t.Errorf("Expected a retry after roughly %s, but actually waited for %s", wanted, got)
			}
		}
	}
}

func TestShouldUseExponentialBackoffIfNoRetryHeader(t *testing.T) {
	MaxRequestDuration = time.Hour * 24

	url := "http://bar"
	retries := 5
	_, clock := setUpAllFailures(url, 501, retries, nil)

	_, _, err := ReadRemoteFile(url, "")
	if err == nil {
		t.Fatal("Expected request to fail with code 501")
	}

	if clock.TimesSlept() != retries {
		t.Fatalf("Expected %d retries, but got %d", retries, clock.TimesSlept())
	}

	var total time.Duration
	for _, p := range clock.SleepPeriods {
		total += p
	}

	delta := time.Millisecond * 100
	lower := (1 << retries) * time.Second
	upper := lower + time.Duration(retries*500)*time.Millisecond
	if total < lower-delta || upper+delta < total {
		t.Fatalf("Expected a total sleep time between %s and %s (%d retries, exponential backoff with fuzzing), but waited %s instead", lower, upper, retries, total)
	}
}

func TestDeadlineExceeded(t *testing.T) {
	MaxRequestDuration = time.Second * 8

	url := "http://bar"
	setUpAllFailures(url, 500, 10, nil)

	_, _, err := ReadRemoteFile(url, "")
	if err == nil {
		t.Fatal("Expected request to fail with code 500")
	}

	wanted := "could not fetch http://bar: unable to complete request to http://bar within 8s"
	got := err.Error()
	if wanted != got {
		t.Fatalf("Expected error %q, but got %q", wanted, got)
	}
}

func TestNoRetryOnPermanentError(t *testing.T) {
	MaxRequestDuration = time.Hour

	url := "http://xyz"
	_, clock := setUpAllFailures(url, 404, 3, nil)

	_, _, err := ReadRemoteFile(url, "")
	if err == nil {
		t.Fatal("Expected request to fail with code 404")
	}

	wanted := "unexpected status code while reading http://xyz: 404"
	got := err.Error()
	if wanted != got {
		t.Fatalf("Expected error %q, but got %q", wanted, got)
	}

	if clock.TimesSlept() > 0 {
		t.Fatalf("Expected no retries for permanent error, but got %d", clock.TimesSlept())
	}
}
