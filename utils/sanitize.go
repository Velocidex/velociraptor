package utils

import (
	"strings"
)

// These functions are used to sanitize a path component for storage
// in the data store. The filestore has to handle any type of file
// name, being able to preserve the filename upon storage and
// retrieval. This means it needs to have perfect round trip
// encoding. One possibility is simply to use base64 encoding to
// preserve filename but this will make the filestore hard to use and
// obfuscate the file names. Note that file names do not have to be
// valid utf8 strings! We could for example encode a hash value, a
// value containing path separators or an un-normalized unicode
// string.

var hexTable = []byte("0123456789ABCDEF")

// We are very conservative about our escaping - only maintain ascii
// printables, numerics and some safe symbols. We do not assume our
// underlying filesystem supports unicode! UTF8 encoding will be hex
// encoded for the filesystem.
func shouldEscape(c byte) bool {
	if 'A' <= c && c <= 'Z' ||
		'a' <= c && c <= 'z' ||
		'0' <= c && c <= '9' {
		return false
	}

	switch c {
	case '-', '_', '.', '~', ' ', '$':
		return false
	}

	return true
}

func SanitizeString(component string) string {
	// Escape components that start with . - these are illegal on
	// windows and can be used for directory traversal. The . byte
	// may appear anywhere else though.
	if len(component) > 0 && component[0:1] == "." {
		return "%2E" + SanitizeString(component[1:])
	}

	// Prevent components from creating names for files that are
	// used internally by the datastore. This will be
	// automatically stripped when decoding.
	if strings.HasSuffix(component, ".db") ||
		strings.HasSuffix(component, "_") {
		component += "_"
	}

	length := len(component)
	if length > 1024 {
		length = 1024
	}

	result := make([]byte, length*4)
	result_idx := 0

	for _, c := range []byte(component) {
		if !shouldEscape(c) {
			result[result_idx] = c
			result_idx += 1
		} else {
			result[result_idx] = '%'
			result[result_idx+1] = hexTable[c>>4]
			result[result_idx+2] = hexTable[c&15]
			result_idx += 3
		}
	}

	return string(result[:result_idx])
}

func unhex(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

func UnsanitizeComponent(component string) string {
	result := make([]byte, len(component))
	var j, i int
	for {
		if i >= len(component) {
			// Strip a single trailing _
			if j > 0 && result[j-1] == '_' {
				j--
			}

			return string(result[:j])
		}

		if component[i] == '%' {
			result[j] = unhex(component[i+1])<<4 | unhex(component[i+2])
			i += 3
			j++
		} else {
			result[j] = component[i]
			i += 1
			j++
		}
	}
}
