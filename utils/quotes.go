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

func MaybeStripWrappingQuotes(path string) string {
	if len(path) >= 2 && path[0] == '\'' && path[len(path)-1] == '\'' {
		return path[1 : len(path)-1]
	}
	return path
}
