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
package glob

import (
	"os"
	"time"

	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Virtual FileInfo for root directory - represent all drives as
// directories.
type VirtualDirectoryPath struct {
	drive string
	// This holds information about the drive.
	data interface{}
}

func NewVirtualDirectoryPath(path string, data interface{}) *VirtualDirectoryPath {
	return &VirtualDirectoryPath{drive: path, data: data}
}

func (self *VirtualDirectoryPath) Name() string {
	return self.drive
}

func (self *VirtualDirectoryPath) Data() interface{} {
	return self.data
}

func (self *VirtualDirectoryPath) Size() int64 {
	return 0
}

func (self *VirtualDirectoryPath) Mode() os.FileMode {
	return os.ModeDir
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

func (self *VirtualDirectoryPath) Atime() utils.TimeVal {
	return utils.TimeVal{}
}

func (self *VirtualDirectoryPath) Mtime() utils.TimeVal {
	return utils.TimeVal{}
}

func (self *VirtualDirectoryPath) Btime() utils.TimeVal {
	return utils.TimeVal{}
}

func (self *VirtualDirectoryPath) Ctime() utils.TimeVal {
	return utils.TimeVal{}
}

// Not supported
func (self *VirtualDirectoryPath) IsLink() bool {
	return false
}

func (self *VirtualDirectoryPath) GetLink() (string, error) {
	return "", errors.New("Not implemented")
}
