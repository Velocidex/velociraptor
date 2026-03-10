package utils

import (
	"fmt"
	"os"
	"strconv"
)

var (
	xwr             = "xwrxwrxwr"
	filemodeInvalid = Wrap(InvalidArgError, "FileMode not valid")
)

func ParseFileMode(in string) (os.FileMode, error) {
	if len(in) == 0 {
		return os.FileMode(0), filemodeInvalid
	}

	// Try to parse it as an octal
	if in[0] == '0' {
		val, err := strconv.ParseUint(in, 0, 32)
		if err != nil {
			return 0, fmt.Errorf("%w: %v", filemodeInvalid, err)
		}
		return os.FileMode(val), nil
	}

	if len(in) > len(xwr) {
		return os.FileMode(0), filemodeInvalid
	}

	val := 0
	in_len := len(in)
	for i := 0; i < 9 && in_len > i; i++ {
		c := in[in_len-i-1]
		if c == '-' {
			continue
		}

		if c != xwr[i] {
			return 0, fmt.Errorf(
				"%w: Invalid Mode specification at index %v: expeting %c got %c",
				filemodeInvalid, i, xwr[i], c)
		}

		val = val | (1 << i)
	}
	return os.FileMode(val), nil
}
