// +build freebsd

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
)

// On Linux we need xstat() support to get birth time.
func (self *OSFileInfo) Btime() time.Time {
	return time.Time{}
}

func (self *OSFileInfo) Mtime() time.Time {
	ts := int64(self._Sys().Mtimespec.Sec)
	return time.Unix(ts, 0)
}

func (self *OSFileInfo) Ctime() time.Time {
	ts := int64(self._Sys().Ctimespec.Sec)
	return time.Unix(ts, 0)
}

func (self *OSFileInfo) Atime() time.Time {
	ts := int64(self._Sys().Atimespec.Sec)
	return time.Unix(ts, 0)
}
