/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
package glob

import (
	"os"
	"time"

	errors "github.com/pkg/errors"
)

// Virtual FileInfo for root directory - represent all drives as
// directories.
type VirtualDirectoryPath struct {
	drive string
	// This holds information about the drive.
	data interface{}
	size int64
	mode os.FileMode
}

func NewVirtualDirectoryPath(path string, data interface{},
	size int64, mode os.FileMode) *VirtualDirectoryPath {
	return &VirtualDirectoryPath{drive: path, data: data, size: size, mode: mode}
}

func (self *VirtualDirectoryPath) Name() string {
	return self.drive
}

func (self *VirtualDirectoryPath) Data() interface{} {
	return self.data
}

func (self *VirtualDirectoryPath) Size() int64 {
	return self.size
}

func (self *VirtualDirectoryPath) Mode() os.FileMode {
	return self.mode
}

func (self *VirtualDirectoryPath) ModTime() time.Time {
	return time.Now()
}

func (self *VirtualDirectoryPath) IsDir() bool {
	return true
}

func (self *VirtualDirectoryPath) Sys() interface{} {
	return nil
}

func (self *VirtualDirectoryPath) FullPath() string {
	return self.drive
}

func (self *VirtualDirectoryPath) Atime() time.Time {
	return time.Time{}
}

func (self *VirtualDirectoryPath) Mtime() time.Time {
	return time.Time{}
}

func (self *VirtualDirectoryPath) Btime() time.Time {
	return time.Time{}
}

func (self *VirtualDirectoryPath) Ctime() time.Time {
	return time.Time{}
}

// Not supported
func (self *VirtualDirectoryPath) IsLink() bool {
	return false
}

func (self *VirtualDirectoryPath) GetLink() (string, error) {
	return "", errors.New("Not implemented")
}
