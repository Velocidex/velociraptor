package utils

import (
	"net/http"
)

// Retrieve the Remote Address from a request in a reverse-proxy compatible way.
func RemoteAddr(req *http.Request, header string) string {
	if len(header) > 0 {
		if addr := req.Header.Get(header); len(addr) > 0 {
			return addr
		}
	}
	return req.RemoteAddr
}
