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
	_ "www.velocidex.com/golang/velociraptor/vql/common"
	_ "www.velocidex.com/golang/velociraptor/vql/filesystem"
	_ "www.velocidex.com/golang/velociraptor/vql/filesystem/ntfs"
	_ "www.velocidex.com/golang/velociraptor/vql/functions"
	_ "www.velocidex.com/golang/velociraptor/vql/golang"
	_ "www.velocidex.com/golang/velociraptor/vql/networking"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers/authenticode"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers/crypto"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers/csv"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers/ese"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers/event_logs"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers/syslog"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers/usn"
	_ "www.velocidex.com/golang/velociraptor/vql/protocols"
	_ "www.velocidex.com/golang/velociraptor/vql/tools"
)
