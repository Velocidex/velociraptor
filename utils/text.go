package utils

import (
	"strings"

	"github.com/mitchellh/go-wordwrap"
)

func Indent(text string, indent int) string {
	prefix := strings.Repeat(" ", indent)
	var res []string
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		res = append(res, prefix+l)
	}

	return strings.Join(res, "\n")
}

func WrapString(text string, width uint) string {
	var res []string
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		res = append(res, wordwrap.WrapString(l, width))
	}
	return strings.Join(res, "\n") + "\n"
}
