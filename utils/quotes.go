package utils

import "strconv"

func Quote(in string) string {
	return strconv.QuoteToASCII(in)
}

func UnQuote(in string) string {
	res, err := strconv.Unquote(in)
	if err != nil {
		return in
	}
	return res
}

// MaybeStripWrappingQuotes removes wrapping single quotes from absolute
// paths produced by macOS Finder's "Copy as Pathname" (Sequoia+).
// Finder only wraps in quotes when the path has spaces and no single
// quotes; paths containing literal single quotes use backslash escaping
// instead, so this is unambiguous.
func MaybeStripWrappingQuotes(path string) string {
	if len(path) >= 3 && path[0] == '\'' && path[len(path)-1] == '\'' && path[1] == '/' {
		return path[1 : len(path)-1]
	}
	return path
}
