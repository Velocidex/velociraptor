/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"

	"www.velocidex.com/golang/velociraptor/actions"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

var (
	debug_flag      = app.Flag("debug", "Enables debug and profile server.").Bool()
	debug_flag_port = app.Flag("debug_port", "Port for the debug server.").
			Default("6060").Int64()
)

// Dumps out the query log
func handleQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, item := range actions.QueryLog.Get() {
		if item.Duration == 0 {
			io.WriteString(w,
				fmt.Sprintf("RUNNING(%v) %v: %v\n",
					time.Now().Sub(item.Start), item.Start, item.Query))
		} else {
			io.WriteString(w,
				fmt.Sprintf("FINISHED(%v) %v: %v\n",
					item.Duration/1e9, item.Start, item.Query))
		}
	}
}

// Dumps out the query log
func handleRunningQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, item := range actions.QueryLog.Get() {
		if item.Duration == 0 {
			io.WriteString(w,
				fmt.Sprintf("RUNNING(%v) %v: %v\n",
					time.Now().Sub(item.Start), item.Start, item.Query))
		}
	}
}

func initDebugServer(config_obj *config_proto.Config) error {
	if *debug_flag {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> debug server on <cyan>http://127.0.0.1:%v/debug/pprof", *debug_flag_port)

		http.HandleFunc("/debug/queries", handleQueries)
		http.HandleFunc("/debug/queries/running", handleRunningQueries)

		go func() {
			log.Println(http.ListenAndServe(
				fmt.Sprintf("127.0.0.1:%d", *debug_flag_port), nil))
		}()
	}
	return nil
}
