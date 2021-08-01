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
package api

import (
	"os"

	"www.velocidex.com/golang/velociraptor/glob"
)

type FileReader interface {
	Read(buff []byte) (int, error)
	Seek(offset int64, whence int) (int64, error)
	Stat() (glob.FileInfo, error)
	Close() error
}

// A file store writer writes files in the filestore. Filestore files
// are not as flexible as real files and only provide a subset of
// functionality. Specifically they can not be over-written - only
// appended to. They can be truncated but only to 0 size.
type FileWriter interface {
	Size() (int64, error)
	Write(data []byte) (int, error)
	Truncate() error
	Close() error
}

type WalkFunc func(urn SafeDatastorePath, info os.FileInfo) error
type FileStore interface {
	ReadFile(filename SafeDatastorePath) (FileReader, error)
	WriteFile(filename SafeDatastorePath) (FileWriter, error)
	StatFile(filename SafeDatastorePath) (os.FileInfo, error)
	ListDirectory(dirname SafeDatastorePath) ([]os.FileInfo, error)
	Walk(root SafeDatastorePath, cb WalkFunc) error
	Delete(filename SafeDatastorePath) error
	Move(src, dest SafeDatastorePath) error

	// The following API can be used with unsafe path components.
	ReadFileComponents(filename UnsafeDatastorePath) (FileReader, error)
	WriteFileComponent(filename UnsafeDatastorePath) (FileWriter, error)
	StatFileComponents(filename UnsafeDatastorePath) (os.FileInfo, error)
	ListDirectoryComponents(filename UnsafeDatastorePath) ([]os.FileInfo, error)
}
