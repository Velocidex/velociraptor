package utils

import (
	"net/url"
	"strings"
)

// Work around issues with https://github.com/golang/go/issues/4013
// and space encoding. This QueryEscape has to be the exact mirror of
// Javascript's decodeURIComponent
func QueryEscape(in string) string {
	res := url.QueryEscape(in)
	return strings.Replace(res, "+", "%20", -1)
}
