package utils

import (
	"fmt"
	"unicode"
)

func Elide(in string, length int) string {
	if len(in) < length {
		return in
	}

	return in[:length] + " ..."
}

func Uniquify(in []string) []string {
	result := make([]string, 0, len(in))
	seen := make(map[string]bool)
	for _, i := range in {
		_, pres := seen[i]
		if pres {
			continue
		}
		seen[i] = true
		result = append(result, i)
	}
	return result
}

func ToString(x interface{}) string {
	switch t := x.(type) {
	case string:
		return t

	case []byte:
		return string(t)

	case error:
		return t.Error()

	case fmt.Stringer:
		return t.String()

	default:
		return fmt.Sprintf("%v", x)
	}
}

// Lower the string in a unicode aware way. This normalizes the
// strings for comparisons.
func ToLower(in string) string {
	var result []rune
	for _, c := range in {
		result = append(result, unicode.ToLower(c))
	}

	return string(result)
}
