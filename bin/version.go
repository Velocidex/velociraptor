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
package main

import (
	"fmt"
	"runtime/debug"

	"github.com/Velocidex/yaml/v2"
	"github.com/alecthomas/kingpin/v2"
	"www.velocidex.com/golang/velociraptor/config"
)

var (
	version = app.Command("version", "Report the binary version and build information.")
)

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == version.FullCommand() {
			res, err := yaml.Marshal(config.GetVersion())
			if err != nil {
				kingpin.FatalIfError(err, "Unable to encode version.")
			}

			fmt.Printf("%v", string(res))

			if *verbose_flag {
				info, ok := debug.ReadBuildInfo()
				if ok {
					fmt.Printf("\n\nBuild Info:\n%v\n", info)
				}
			}

			return true
		}
		return false
	})
}
