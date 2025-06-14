package http

import (
	"net/http"
	"sync"

	"github.com/leaktk/leaktk/pkg/version"
)

var once sync.Once
var client *http.Client

// NewClient creates an http client with preferred configuration
func NewClient() *http.Client {
	once.Do(func() {
		client = &http.Client{
			Transport: &customRoundTripper{
				rt: http.DefaultTransport,
			},
		}
	})

	return client
}

type customRoundTripper struct {
	rt http.RoundTripper
}

func (rt *customRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", version.GlobalUserAgent)

	return rt.rt.RoundTrip(req)
}
