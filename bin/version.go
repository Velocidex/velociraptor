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
package main

import (
	"fmt"

	"github.com/Velocidex/yaml/v2"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	version = app.Command("version", "Report client version.")
)

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == version.FullCommand() {
			config_obj := load_config_or_default()
			res, err := yaml.Marshal(config_obj.Version)
			if err != nil {
				kingpin.FatalIfError(err, "Unable to encode version.")
			}

			fmt.Printf("%v", string(res))
			return true
		}
		return false
	})
}
