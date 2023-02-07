package rpc

import (
	"net/http"
	"sync"
)

const (
	maxRequestContentLength = 1024 * 1024 * 5
	contentType             = "application/json"
)

// https://www.jsonrpc.org/historical/json-rpc-over-http.html#id13
var acceptedContentTypes = []string{contentType, "application/json-rpc", "application/jsonrequest"}

type httpConn struct {
	client *http.Client
	url string
	closeOnce sync.Once
	closeCh chan interface{}
	mu sync.Mutex //protects headers
	headers http.Header
	auth HTTPAuth
}