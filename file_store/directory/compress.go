/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
package directory

import (
	"fmt"
	"io"
	"os"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/glob"
)

// GzipReader is a FileReader from compressed files.
type GzipReader struct {
	zip_fd       io.Reader
	backing_file *os.File

	full_path string
}

func (self *GzipReader) Read(buff []byte) (int, error) {
	return self.zip_fd.Read(buff)
}

func (self *GzipReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		if offset == 0 {
			return 0, nil
		}

	}
	return 0, fmt.Errorf(
		"Seeking to %v (%v) not supported on compressed files.",
		offset, whence)
}

func (self GzipReader) Stat() (glob.FileInfo, error) {
	stat, err := self.backing_file.Stat()
	if err != nil {
		return nil, err
	}
	return api.NewFileInfoAdapter(stat, self.full_path, nil), nil
}

func (self *GzipReader) Close() error {
	return self.backing_file.Close()
}
