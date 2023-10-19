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
	"html"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"path"
	"regexp"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/actions"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/golang"
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

func handleProfile(config_obj *config_proto.Config) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		profile_type, _ := url.QueryUnescape(path.Base(r.URL.Path))

		// The root level shows all profile information.
		if profile_type == "profile" || profile_type == "all" {
			profile_type = "."
		} else {
			// Take the profile type as a literal string not a regex
			profile_type = regexp.QuoteMeta(profile_type)
		}

		builder := services.ScopeBuilder{
			Config:     config_obj,
			ACLManager: acl_managers.NullACLManager{},
			Env:        ordereddict.NewDict(),
		}

		manager, err := services.GetRepositoryManager(config_obj)
		if err != nil {
			return
		}
		scope := manager.BuildScope(builder)
		defer scope.Close()

		plugin := &golang.ProfilePlugin{}
		for row := range plugin.Call(r.Context(), scope, ordereddict.NewDict().
			Set("type", profile_type)) {
			serialized, _ := json.Marshal(row)
			serialized = append(serialized, '\n')
			w.Write(serialized)
		}
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	w.Write([]byte(`
<html><body>
<h1>Debug Server</h1>
<ul>
<li><a href="/debug/queries">Show all queries</a></li>
<li><a href="/debug/queries/running">Show currently running queries</a></li>
<li><a href="/debug/profile/all">Show all profile items</a></li>
`))

	for _, i := range debug.GetProfileWriters() {
		w.Write([]byte(fmt.Sprintf(`
<li><a href="/debug/profile/%s">%s</a></li>`,
			url.QueryEscape(i.Name),
			html.EscapeString(i.Description))))
	}

	w.Write([]byte(`</body></html>`))
}

func initDebugServer(config_obj *config_proto.Config) error {
	if *debug_flag {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> debug server on <cyan>http://127.0.0.1:%v/debug/pprof", *debug_flag_port)

		http.HandleFunc("/debug", handleIndex)
		http.HandleFunc("/debug/queries", handleQueries)
		http.HandleFunc("/debug/profile/", handleProfile(config_obj))
		http.HandleFunc("/debug/queries/running", handleRunningQueries)

		// Switch off the debug flag so we do not run this again. (The
		// GUI runs this function multiple times).
		*debug_flag = false

		go func() {
			log.Println(http.ListenAndServe(
				fmt.Sprintf("127.0.0.1:%d", *debug_flag_port), nil))
		}()
	}
	return nil
}
