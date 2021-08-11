// +build server_vql

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

// This is a do nothing package which just forces an import of the
// various VQL plugin directories. The plugins will register
// themselves.

import (
	_ "www.velocidex.com/golang/velociraptor/vql/server"
	_ "www.velocidex.com/golang/velociraptor/vql/server/clients"
	_ "www.velocidex.com/golang/velociraptor/vql/server/downloads"
	_ "www.velocidex.com/golang/velociraptor/vql/server/favorites"
	_ "www.velocidex.com/golang/velociraptor/vql/server/hunts"
	_ "www.velocidex.com/golang/velociraptor/vql/server/notebooks"
	_ "www.velocidex.com/golang/velociraptor/vql/server/timelines"
	_ "www.velocidex.com/golang/velociraptor/vql/server/users"
)
