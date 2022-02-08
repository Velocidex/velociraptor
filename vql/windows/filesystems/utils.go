package filesystems

import "strings"

// We want to show the entire device as one name so we need to escape
// \\ characters so they are not interpreted as a path separator.
func escape(path string) string {
	result := strings.Replace(path, "\\", "%5c", -1)
	return strings.Replace(result, "/", "%2f", -1)
}

func unescape(path string) string {
	result := strings.Replace(path, "%5c", "\\", -1)
	return strings.Replace(result, "%2f", "/", -1)
}
