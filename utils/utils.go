package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"www.velocidex.com/golang/vfilter"
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

func hard_wrap(text string, colBreak int) string {
	text = strings.TrimSpace(text)
	wrapped := ""
	var i int
	for i = 0; len(text[i:]) > colBreak; i += colBreak {

		wrapped += text[i:i+colBreak] + "\n"

	}
	wrapped += text[i:]

	return wrapped
}

func Stringify(value interface{}, scope *vfilter.Scope) string {
	switch t := value.(type) {
	case vfilter.StringProtocol:
		return t.ToString(scope)
	case fmt.Stringer:
		return hard_wrap(t.String(), 30)
	case []byte:
		return hard_wrap(string(t), 30)
	case string:
		return hard_wrap(t, 30)
	default:
		if k, err := json.Marshal(value); err == nil {
			return hard_wrap(string(k), 30)
		}
	}
	return ""
}

func SlicesEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for idx, a_item := range a {
		if a_item != b[idx] {
			return false
		}
	}

	return true
}
