// +build windows

package utils

import (
	"io/ioutil"
	"os"

	"golang.org/x/sys/windows/registry"
)

func ReadDir(dirname string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(dirname)
}

func ExpandEnv(path string) string {
	path = os.ExpandEnv(path)
	expanded_path, err := registry.ExpandString(path)
	if err != nil {
		return path
	}

	return expanded_path
}
