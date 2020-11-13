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
package plugins

import (
	_ "www.velocidex.com/golang/velociraptor/vql/windows"
	_ "www.velocidex.com/golang/velociraptor/vql/windows/authenticode"
	_ "www.velocidex.com/golang/velociraptor/vql/windows/dns"
	_ "www.velocidex.com/golang/velociraptor/vql/windows/etw"
	_ "www.velocidex.com/golang/velociraptor/vql/windows/filesystems"
	_ "www.velocidex.com/golang/velociraptor/vql/windows/process"
	_ "www.velocidex.com/golang/velociraptor/vql/windows/wmi"
)
