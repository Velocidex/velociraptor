// +build darwin

package utils

import (
	"io/ioutil"
	"os"
)

func ReadDir(dirname string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(dirname)
}

func ReadDirUnsorted(dirname string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(dirname)
}

func ExpandEnv(path string) string {
	return os.ExpandEnv(path)
}
