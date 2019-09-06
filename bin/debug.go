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
	"log"
	"net/http"
	_ "net/http/pprof"

	logging "www.velocidex.com/golang/velociraptor/logging"
)

var (
	debug_flag = app.Flag("debug", "Enables debug and profile server.").Bool()
)

func doDebug() {
	config_obj := get_config_or_default()
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Starting debug server on http://0.0.0.0:6060/debug/pprof")

	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()
}

func init() {
	// Add this first to ensure it always runs.
	command_handlers = append([]CommandHandler{
		func(command string) bool {
			if *debug_flag {
				doDebug()
			}
			return false
		},
	}, command_handlers...)
}
