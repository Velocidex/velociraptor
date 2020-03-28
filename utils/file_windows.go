// +build windows

package utils

import (
	"io/ioutil"
	"os"
)

func ReadDir(dirname string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(dirname)
}
