package goldie

import (
	"regexp"
	"strings"
)

func RemoveLines(regex string, data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	new_lines := make([]string, 0, len(lines))
	re := regexp.MustCompile(regex)
	for _, l := range lines {
		// Remove lines that match the regexp
		if !re.MatchString(l) {
			new_lines = append(new_lines, l)
		}
	}

	return []byte(strings.Join(new_lines, "\n"))
}

func ReplaceLines(regex string, replace string, data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	new_lines := make([]string, 0, len(lines))
	re := regexp.MustCompile(regex)
	for _, l := range lines {
		// Remove lines that match the regexp
		l = re.ReplaceAllString(l, replace)
		new_lines = append(new_lines, l)
	}

	return []byte(strings.Join(new_lines, "\n"))
}
