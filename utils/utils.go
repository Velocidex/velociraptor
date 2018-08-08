package utils

import (
	"reflect"
	"strings"
)

func InString(hay *[]string, needle string) bool {
	for _, x := range *hay {
		if x == needle {
			return true
		}
	}

	return false
}

func IsNil(a interface{}) bool {
	defer func() { recover() }()
	return a == nil || reflect.ValueOf(a).IsNil()
}

// Massage a windows path into a standard form:
// \ are replaced with /
// Drive letters are preceeded with /
// Example: c:\windows ->  /c:/windows
func Normalize_windows_path(filename string) string {
	filename = strings.Replace(filename, "\\", "/", -1)
	if !strings.HasPrefix(filename, "/") {
		filename = "/" + filename
	}
	return filename
}
