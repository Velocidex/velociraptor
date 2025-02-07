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
	"context"
	"fmt"
	"os"

	errors "github.com/go-errors/errors"
)

// https://stackoverflow.com/questions/21060945/simple-way-to-copy-a-file-in-golang

// CopyFile copies a file from src to dst. If src and dst files exist,
// and are the same, then return success. Otherise, copy the file
// contents from src to dst.
func CopyFile(ctx context.Context,
	src, dst string, mode os.FileMode) (err error) {
	sfi, err := os.Stat(src)
	if err != nil {
		return errors.Wrap(err, 0)
	}
	if !sfi.Mode().IsRegular() {
		// cannot copy non-regular files (e.g., directories,
		// symlinks, devices, etc.)
		return fmt.Errorf("CopyFile: non-regular source file %s (%q)", sfi.Name(), sfi.Mode().String())
	}

	dfi, err := os.Stat(dst)
	if err != nil {
		// File may not exist yet so this is not an error.
		if !os.IsNotExist(err) {
			return errors.Wrap(err, 0)
		}
	} else {

		if !(dfi.Mode().IsRegular()) {
			return fmt.Errorf(
				"CopyFile: non-regular destination file %s (%q)",
				dfi.Name(), dfi.Mode().String())
		}

		// Files are the same - it is not an error but there
		// is nothing else to do.
		if os.SameFile(sfi, dfi) {
			return nil
		}
	}

	// This may not work if the files are on different filesystems
	// or the filesystem does not support it.
	return copyFileContents(ctx, src, dst, mode)
}

// copyFileContents copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file.
func copyFileContents(ctx context.Context,
	src, dst string, mode os.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, 0)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return errors.Wrap(err, 0)
	}

	defer func() {
		cerr := out.Close()
		if err == nil {
			err = errors.Wrap(cerr, 0)
		}
	}()

	if _, err = Copy(ctx, out, in); err != nil {
		return errors.Wrap(err, 0)
	}

	return out.Sync()
}

func ReadDirNames(dirname string) ([]string, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(-1)
	f.Close()

	return names, err
}
