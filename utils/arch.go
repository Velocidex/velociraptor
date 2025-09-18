package utils

import (
	"os"
	"runtime"
	"strings"
)

func GetArch() string {
	res := runtime.GOARCH

	// On windows, detect if we are running in Wow64
	if runtime.GOOS == "windows" {
		proc_arch := os.Getenv("PROCESSOR_ARCHITECTURE")
		if proc_arch != "" {
			res = proc_arch

			if proc_arch == "x86" {
				wow_arch := os.Getenv("PROCESSOR_ARCHITEW6432")
				if wow_arch == "AMD64" {
					res = "wow64"
				}
			}
		}
	}

	return strings.ToLower(res)
}
