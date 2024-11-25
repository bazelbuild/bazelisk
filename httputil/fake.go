package httputil

import (
	"bytes"
	"io"
	"net/http"
)

// FakeTransport represents a fake http.Transport that returns prerecorded responses.
type FakeTransport struct {
	responses map[string]*responseCollection

	RequestedURLs []string
}

// NewFakeTransport creates a new FakeTransport instance without any responses.
func NewFakeTransport() *FakeTransport {
	return &FakeTransport{
		responses: make(map[string]*responseCollection),
	}
}

func (ft *FakeTransport) responseCollection(url string) *responseCollection {
	if _, ok := ft.responses[url]; !ok {
		ft.responses[url] = &responseCollection{}
	}
	return ft.responses[url]
}

// AddResponse stores a fake HTTP response for the given URL.
func (ft *FakeTransport) AddResponse(url string, status int, body string, headers map[string]string) {
	ft.responseCollection(url).Add(createResponse(status, body, headers), nil)
}

// AddError stores a error for the given URL.
func (ft *FakeTransport) AddError(url string, err error) {
	ft.responseCollection(url).Add(nil, err)

}

// RoundTrip returns a prerecorded response to the given request, if one exists. Otherwise its response indicates 404 - not found.
func (ft *FakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ft.RequestedURLs = append(ft.RequestedURLs, req.URL.String())
	if responses, ok := ft.responses[req.URL.String()]; ok {
		return responses.Next()
	}
	return notFound(), nil
}

type responseCollection struct {
	all  []responseError
	next int
}

func (rc *responseCollection) Add(resp *http.Response, err error) {
	rc.all = append(rc.all, responseError{resp: resp, err: err})
}

func (rc *responseCollection) Next() (*http.Response, error) {
	if rc.next >= len(rc.all) {
		return notFound(), nil
	}
	rc.next++
	next := rc.all[rc.next-1]
	return next.resp, next.err
}

func createResponse(status int, body string, headers map[string]string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     transformHeaders(headers),
	}
}

func transformHeaders(original map[string]string) http.Header {
	result := make(map[string][]string)
	for k, v := range original {
		result[k] = []string{v}
	}
	return result
}

func notFound() *http.Response {
	return createResponse(http.StatusNotFound, "", nil)
}

type responseError struct {
	resp *http.Response
	err  error
}
