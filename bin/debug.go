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
	"log"
	"net/http"
	_ "net/http/pprof"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

var (
	debug_flag      = app.Flag("debug", "Enables debug and profile server.").Bool()
	debug_flag_port = app.Flag("debug_port", "Port for the debug server.").
			Default("6060").Int64()
)

func initDebugServer(config_obj *config_proto.Config) error {
	if *debug_flag {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> debug server on <red>http://127.0.0.1:%v/debug/pprof", *debug_flag_port)

		go func() {
			log.Println(http.ListenAndServe(
				fmt.Sprintf("127.0.0.1:%d", *debug_flag_port), nil))
		}()
	}
	return nil
}
