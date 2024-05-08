package utils

import (
	"os"
	"regexp"
)

var (
	expand_regex = regexp.MustCompile("%([a-zA-Z_0-9]+)%")
)

func ExpandEnv(v string) string {
	// Support windows style % % expansions by converting %VAR%
	// pattern to ${VAR}
	v = expand_regex.ReplaceAllString(v, "$${$1}")

	// Custom getenv function to allow bare $ to be used anywhere.
	return os.Expand(v, getenv)
}

func getenv(v string) string {
	// Allow $ to be escaped (#850) by doubling up $
	switch v {
	case "$":
		return "$"
	}
	return os.Getenv(v)
}
