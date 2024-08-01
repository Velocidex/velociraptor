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
