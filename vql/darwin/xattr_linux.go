package darwin

import "strings"

const prefix = "user."

// Strip "user." prefix on Linux.
func stripPrefix(s []string) []string {
	for i, a := range s {
		if strings.HasPrefix(a, prefix) {
			s[i] = a[5:]
		}
	}
	return s
}
