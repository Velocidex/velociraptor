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
package utils

import (
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
)

func Debug(arg interface{}) {
	spew.Dump(arg)
}

func DlvBreak() {
	if false {
		fmt.Printf("Break")
	}
}

func DebugToFile(filename, format string, v ...interface{}) {
	fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		panic(err)
	}
	defer fd.Close()

	fd.Seek(0, os.SEEK_END)
	fd.Write([]byte(fmt.Sprintf(format, v...) + "\n"))
}
