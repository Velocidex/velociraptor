package psutils

import (
	"fmt"
	"os"
)

// PathExistsWithContents returns the filename exists and it is not
// empty
func PathExistsWithContents(filename string) bool {
	info, err := os.Stat(filename)
	if err != nil {
		return false
	}
	return info.Size() > 4 // at least 4 bytes
}

func GetHostProc(pid int32) string {
	return fmt.Sprintf("%s/%d", GetEnv("HOST_PROC", "/proc"), pid)
}

func GetEnv(key string, dfault string) string {
	value := os.Getenv(key)
	if value == "" {
		return dfault
	}

	return value
}

func ByteToString(orig []byte) string {
	n := -1
	l := -1
	for i, b := range orig {
		// skip left side null
		if l == -1 && b == 0 {
			continue
		}
		if l == -1 {
			l = i
		}

		if b == 0 {
			break
		}
		n = i + 1
	}
	if n == -1 {
		return string(orig)
	}
	return string(orig[l:n])
}
