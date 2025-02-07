//go:build linux || darwin || freebsd
// +build linux darwin freebsd

/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package utils

import (
	"io"
	"os"
	"sort"

	"golang.org/x/sys/unix"
)

// Like io/utils but more robust - return as many files as we can. See
// https://github.com/golang/go/issues/27416

// Copied and modified from os/dir_unix.go
func readdir(f *os.File, n int) (fi []os.FileInfo, err error) {
	dirname := f.Name()
	if dirname == "" {
		dirname = "."
	}
	names, err := readdirnames(f, n)

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

func ReadDirUnsorted(dirname string) ([]os.FileInfo, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	list, err := readdir(f, -1)
	f.Close()

	return list, err
}

func CheckDirWritable(dirname string) error {
	return unix.Access(dirname, unix.W_OK)
}
