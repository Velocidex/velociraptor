package utils

import (
	"regexp"
	"strings"
)

// Split the path into components. Note that since registry keys and
// value may contain path separators in their name, we need to ensure
// such names are escaped using quotes. For example:
// HKEY_USERS\S-1-5-21-546003962-2713609280-610790815-1003\Software\Microsoft\Windows\CurrentVersion\Run\"c:\windows\system32\mshta.exe"
var component_quoted_regex = regexp.MustCompile(`^"([^"\\/]*(?:\\.[^"\\/]*)*)"`)
var component_unquoted_regex = regexp.MustCompile(`^([^\\/]+)([\\/]?|$)`)

func SplitComponents(path string) []string {
	var components []string
	for len(path) > 0 {
		match := component_quoted_regex.FindStringSubmatch(path)
		if len(match) > 0 {
			if len(match[1]) > 0 {
				components = append(components, match[1])
			}
			path = path[len(match[0]):]
			continue
		}
		match = component_unquoted_regex.FindStringSubmatch(path)
		if len(match) > 0 {
			if len(match[1]) > 0 {
				components = append(components, match[1])
			}
			path = path[len(match[0]):]
			continue
		}

		// This should never happen!
		return strings.Split(path, "\\")
	}
	return components
}
