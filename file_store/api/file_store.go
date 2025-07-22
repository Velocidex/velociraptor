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
package api

import (
	"os"
)

type FileReader interface {
	Read(buff []byte) (int, error)
	Seek(offset int64, whence int) (int64, error)
	Stat() (FileInfo, error)
	Close() error
}

// A file store writer writes files in the filestore. Filestore files
// are not as flexible as real files and only provide a subset of
// functionality. Specifically they can not be over-written - only
// appended to. They can be truncated but only to 0 size.
type FileWriter interface {
	Size() (int64, error)
	Write(data []byte) (int, error)

	// WriteCompressed writes an already compressed buffer.
	WriteCompressed(
		data []byte, // The compressed data
		offset uint64, // The offset of the chunk in the logical file.
		uncompressed_size int, // The size of the data in the logical file
	) (int, error)
	Truncate() error
	Close() error

	// Allow the data to be updated in place.
	Update(data []byte, offset int64) error

	// Force the writer to be flushed to disk immediately.
	Flush() error
}

type FileInfo interface {
	os.FileInfo
	PathSpec() FSPathSpec
}

type FileStore interface {
	ReadFile(filename FSPathSpec) (FileReader, error)

	// Async write - same as WriteFileWithCompletion with BackgroundWriter
	WriteFile(filename FSPathSpec) (FileWriter, error)

	// Completion function will be called when the file is committed.
	WriteFileWithCompletion(
		filename FSPathSpec,
		completion func()) (FileWriter, error)

	StatFile(filename FSPathSpec) (FileInfo, error)
	ListDirectory(dirname FSPathSpec) ([]FileInfo, error)
	Delete(filename FSPathSpec) error
	Move(src, dest FSPathSpec) error

	// Clean up any filestore connections
	Close() error
}

type Flusher interface {
	Flush()
}
