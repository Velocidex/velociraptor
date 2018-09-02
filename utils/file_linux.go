package utils

import (
	"io"
	"os"
	"sort"
)

// Like io/utils but more robust - return as many files as we can. See
// https://github.com/golang/go/issues/27416

// Copied and modified from os/dir_unix.go
func readdir(f *os.File, n int) (fi []os.FileInfo, err error) {
	dirname := f.Name()
	if dirname == "" {
		dirname = "."
	}
	names, err := f.Readdirnames(n)
	fi = make([]os.FileInfo, 0, len(names))
	for _, filename := range names {
		fip, lerr := os.Lstat(dirname + "/" + filename)
		if lerr != nil {
			// Ignore Lstat errors but keep going.
			continue
		}
		fi = append(fi, fip)
	}
	if len(fi) == 0 && err == nil && n > 0 {
		// Per File.Readdir, the slice must be non-empty or err
		// must be non-nil if n > 0.
		err = io.EOF
	}
	return fi, err
}

func ReadDir(dirname string) ([]os.FileInfo, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	list, err := readdir(f, -1)
	f.Close()

	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	return list, err
}
