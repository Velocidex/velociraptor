package utils

import (
	"bytes"
	"strings"
)

type AutoDetectMime bool

// Only handle the types we usually handle in the GUI
func GetMimeString(buffer []byte, detect_mime AutoDetectMime) string {
	if detect_mime && len(buffer) > 8 {
		if 0 == bytes.Compare(
			[]byte("\x89\x50\x4E\x47\x0D\x0A\x1A\x0A"), buffer[:8]) {
			return "image/png"
		}

		if len(buffer) > 20 && strings.HasPrefix(
			strings.ToLower(string(buffer[:20])), `<svg version=`) {
			return "image/svg+xml"
		}

	}
	return "binary/octet-stream"
}
