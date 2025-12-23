package utils

import (
	"strings"
	"unicode/utf8"
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
	length := len(component)
	if length == 0 {
		return ""
	}

	// Escape components that start with . - these are illegal on
	// windows and can be used for directory traversal. The . byte
	// may appear anywhere else though.
	if component[0] == '.' {
		return "%2E" + SanitizeString(component[1:])
	}

	// Escape components that start with # as the data store
	// represents those as hashes.
	if component[0] == '#' {
		return "%23" + SanitizeString(component[1:])
	}

	// Windows can not have a trailing "." instead swallowing it
	// completely.
	if component[length-1] == '.' {
		return SanitizeString(component[:length-1]) + "%2E"
	}

	// Prevent components from creating names for files that are
	// used internally by the datastore. This will be
	// automatically stripped when decoding.
	if strings.HasSuffix(component, ".db") ||
		strings.HasSuffix(component, "_") {
		component += "_"
	}

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

		if result_idx > len(result)-1 {
			break
		}
	}

	return string(result[:result_idx])
}

// This assumes c is an ascii rune!
func escapeChar(c rune) []rune {
	return []rune{'%', rune(hexTable[c>>4]), rune(hexTable[c&15])}
}

// A less rigorous escaper which is suitable for zip paths. We assume
// Zip paths may contain UTF8 unicode but can not contain the
// following characters (https://en.wikipedia.org/wiki/Filename):
// 1. The following will always be escaped: /\*?:|<>%
// 2. Non printables
// 3. A "." or " " at the end or the start of the file.
//
// Characters that do not fit into these rules will be escaped using URL encoding.
func SanitizeStringForZip(component string) string {
	result := make([]rune, 0, len(component))

	escape := func(char rune) {
		result = append(result, escapeChar(char)...)
	}

	for pos, char := range component {
		if pos == 0 && (char == '.' || char == ' ') {
			escape(char)
			continue
		}

		// This is just some binary data that is not UTF8 - escape it
		// so it can be preserved
		if char == utf8.RuneError {
			if pos < len(component) {
				c := component[pos]
				escape(rune(c))
			}
			continue
		}

		switch char {
		case '?', '"', '*', ':', '|', '<', '>', '%', '/', '\\':
			escape(char)

		default:
			result = append(result, char)
		}

		// Maximum length of one component.
		if pos > 1024 {
			break
		}
	}

	length := len(result)
	if length == 0 {
		return ""
	}

	// Windows can not have a trailing "." instead swallowing it
	// completely.
	suffix := result[length-1]
	if suffix == '.' || suffix == ' ' {
		result = result[:length-1]
		escape(suffix)
	}

	return string(result)
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
			// A % escape sequece (eg %0d)
			if i+2 < len(component) {
				result[j] = unhex(component[i+1])<<4 | unhex(component[i+2])
				i += 3
				j++
			} else {
				// Skip trailing % - sometimes this is added by
				// windows for files with no extension (foo. -> foo.%)
				i++
			}
		} else {
			result[j] = component[i]
			i += 1
			j++
		}
	}
}

func UnsanitizeComponentForZip(component string) string {
	result := make([]byte, len(component))
	var j, i int
	for {
		if i >= len(component) {
			return string(result[:j])
		}

		if component[i] == '%' {
			// A % escape sequece (eg %0d)
			if i+2 < len(component) {
				result[j] = unhex(component[i+1])<<4 | unhex(component[i+2])
				i += 3
				j++
			} else {
				// Skip trailing % - sometimes this is added by
				// windows for files with no extension (foo. -> foo.%)
				i++
			}
		} else {
			result[j] = component[i]
			i += 1
			j++
		}
	}
}
