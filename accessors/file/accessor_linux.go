// +build linux

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

package file

import (
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
)

// On Linux we need xstat() support to get birth time.
func (self *OSFileInfo) Btime() time.Time {
	return time.Time{}
}

func (self *OSFileInfo) Mtime() time.Time {
	ts := int64(self._Sys().Mtim.Sec)
	return time.Unix(ts, 0)
}

func (self *OSFileInfo) Ctime() time.Time {
	ts := int64(self._Sys().Ctim.Sec)
	return time.Unix(ts, 0)
}

func (self *OSFileInfo) Atime() time.Time {
	ts := int64(self._Sys().Atim.Sec)
	return time.Unix(ts, 0)
}

func init() {
	accessors.Register("file", &OSFileSystemAccessor{
		root: accessors.NewLinuxOSPath(""),
	}, `Access files using the operating system's API. Does not allow access to raw devices.`)

	accessors.Register("raw_file", &OSFileSystemAccessor{
		root:             accessors.NewLinuxOSPath(""),
		allow_raw_access: true,
	}, `Access files using the operating system's API. Also allow access to raw devices.`)

	// On Linux the auto accessor is the same as file.
	accessors.Register("auto", &OSFileSystemAccessor{
		root: accessors.NewLinuxOSPath(""),
	}, `Access the file using the best accessor possible. On windows we fall back to NTFS parsing in case the file is locked or unreadable.`)
}
