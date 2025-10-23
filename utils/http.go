package utils

import "net/http"

// Create a HTTPClient with superpowers to be used everywhere.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}
