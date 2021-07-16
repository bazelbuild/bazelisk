package httputil

import (
	"bytes"
	"io/ioutil"
	"net/http"
)

type FakeTransport struct {
	responses map[string]*responseCollection
}

func NewFakeTransport() *FakeTransport {
	return &FakeTransport{
		responses: make(map[string]*responseCollection),
	}
}

func (ft *FakeTransport) AddResponse(url string, status int, body string, headers map[string]string) {
	if _, ok := ft.responses[url]; !ok {
		ft.responses[url] = &responseCollection{}
	}

	ft.responses[url].Add(createResponse(status, body, headers))
}

func (ft *FakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if responses, ok := ft.responses[req.URL.String()]; ok {
		return responses.Next(), nil
	}
	return notFound(), nil
}

type responseCollection struct {
	all  []*http.Response
	next int
}

func (rc *responseCollection) Add(resp *http.Response) {
	rc.all = append(rc.all, resp)
}

func (rc *responseCollection) Next() *http.Response {
	if rc.next >= len(rc.all) {
		return notFound()
	}
	rc.next++
	return rc.all[rc.next-1]
}

func createResponse(status int, body string, headers map[string]string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       ioutil.NopCloser(bytes.NewBufferString(body)),
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
