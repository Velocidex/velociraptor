/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2024 Rapid7 Inc.

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
	"www.velocidex.com/golang/vfilter"
)

var (
	debug_flag      = app.Flag("debug", "Enables debug and profile server.").Bool()
	debug_flag_port = app.Flag("debug_port", "Port for the debug server.").
			Default("6060").Int64()
)

// Dumps out the query log
func handleQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rows := []vfilter.Row{}
	for _, item := range actions.QueryLog.Get() {
		row := ordereddict.NewDict().
			Set("Status", "").
			Set("Duration", "").
			Set("Started", item.Start).
			Set("Query", item.Query)
		if item.Duration == 0 {
			row.Update("Status", "RUNNING").
				Update("Duration", time.Now().Sub(item.Start))
		} else {
			row.Update("Status", "FINISHED").
				Update("Duration", item.Duration/1e9)
		}
		rows = append(rows, row)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	serialized, _ := json.Marshal(rows)
	w.Write(serialized)
}

// Dumps out the query log
func handleRunningQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rows := []vfilter.Row{}
	for _, item := range actions.QueryLog.Get() {
		if item.Duration == 0 {
			rows = append(rows, ordereddict.NewDict().
				Set("Status", "RUNNING").
				Set("Duration", time.Now().Sub(item.Start)).
				Set("Started", item.Start).
				Set("Query", item.Query))
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	serialized, _ := json.Marshal(rows)
	w.Write(serialized)
}

const HtmlTemplate = `
<html>
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
    <link rel="stylesheet" href="https://cdn.datatables.net/1.13.7/css/jquery.dataTables.min.css" crossorigin="anonymous">
    <script src="https://code.jquery.com/jquery-3.7.0.js"></script>
    <script src="https://cdn.datatables.net/1.13.7/js/jquery.dataTables.min.js"></script>
    <style>
      li.object {
        list-style: none;
      }

      table.dataTable {
        width: 100%%;
        display: block;
        overflow: auto;
        height: calc(100vh - 250px);
      }

      li.object .key {
        display: inline-grid;
        color: red;
        width: 100px;
      }
      button.btn {
        margin: 5px;
      }
    </style>
</head>
<body>
<h2>%s: %s</h2>
<table id="table" class="display" style="width:100%%">
<script>

let url = window.location.href ;
let raw_json = url.substring(0, url.lastIndexOf('/')) + "/json";

function render( data ) {
  if(typeof data === "string") {
      return data;
  }

  if(typeof data !== "object") {
      return JSON.stringify(data);
  }

  var items = $("<ul/>");
  $.each( data, function( key, val ) {
    val = render(val);
    items.append($("<li class='object'/>").
          append($("<span class='key'/>").append(key)).
          append($("<span class='value'>").append(val)));
  });

  return items.prop('outerHTML');;
};

function loadData(loadingFunction) {
    $.ajax({
        type: 'GET',
        dataType: 'json',
        url: raw_json,
        success: function(d) {
            if (d.length > 0) {
              let seen = d[0];
              for(let i=0;i<d.length;i++) {
                   seen = {...seen, ...d[i]};
              }

              let columns = Object.keys(seen)
              let columns_defs = columns.map(
                 function(name) {return {
                     data: function(row, type, set, meta ) {
                         let x = row[name];
                         if (typeof x === "string" ) {
                             return x;
                         }
                         if (typeof x === "undefined") {
                             return "";
                         }
                         return x;
                     },
                     render: render,
                     title: name,
              }});

              loadingFunction(d, columns_defs);
            }
        }
    });
}

var dataTable;

$(document).ready(function() {
    $("body").prepend($("<a/>").attr("href", raw_json).append(
        "<button class='btn'>Raw JSON</button>"));
    $("body").prepend($("<button class='btn'/>").on("click", function() {
     loadData(function(data, columns_defs) {
        dataTable.clear();
        dataTable.rows.add(data).draw();
     });
    }).append("Reload"));
    loadData(function(data, columns_defs) {
              dataTable = $('#table').DataTable({
                data: data,
                columns: columns_defs,
              });
    });
});
</script>
</body>
</html>
`

func maybeRenderHTML(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if path.Base(r.URL.Path) == "html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")

			profile_type, _ := url.QueryUnescape(
				path.Base(path.Dir(r.URL.Path)))

			description := ""
			for _, i := range debug.GetProfileWriters() {
				if i.Name == profile_type {
					description = i.Description
				}
			}

			switch profile_type {
			case "running":
				description = "Show currently running queries"
			case "queries":
				description = "Show recent queries"
			case "all":
				description = "Show all profile items"
			}

			w.Write([]byte(fmt.Sprintf(
				HtmlTemplate, profile_type, description)))
		} else {
			handler(w, r)
		}
	}
}

func handleProfile(config_obj *config_proto.Config) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		profile_type, _ := url.QueryUnescape(path.Base(r.URL.Path))

		format := "jsonl"
		if profile_type == "json" {
			format = "json"
			profile_type, _ = url.QueryUnescape(path.Base(path.Dir(r.URL.Path)))
		}

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
		rows := []vfilter.Row{}
		for row := range plugin.Call(
			r.Context(), scope, ordereddict.NewDict().
				Set("type", profile_type)) {
			if format == "jsonl" {
				serialized, _ := json.Marshal(row)
				serialized = append(serialized, '\n')
				w.Write(serialized)

			} else {
				rows = append(rows, row)
			}
		}

		if format == "json" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			serialized, _ := json.Marshal(rows)
			w.Write(serialized)
		}
	}
}

func handleIndex(config_obj *config_proto.Config) func(
	w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		w.Write([]byte(`
<html><body>
<h1>Debug Server</h1>
<ul>
<li><a href="/debug/pprof">Show built in Go Profiles</a></li>
<li><a href="/debug/queries/html">Show all queries</a></li>
<li><a href="/debug/queries/running/html">Show currently running queries</a></li>
<li><a href="/debug/profile/all/html">Show all profile items</a></li>
`))

		if config_obj.Monitoring != nil && config_obj.GUI != nil {
			metrics_url := config_obj.Monitoring.MetricsUrl
			if metrics_url == "" {
				metrics_url = fmt.Sprintf("http://%v:%v/metrics",
					config_obj.Monitoring.BindAddress,
					config_obj.Monitoring.BindPort)
			}

			w.Write([]byte(fmt.Sprintf(
				"<li><a href=\"%s\">Metrics</a></li>\n",
				url.QueryEscape(metrics_url))))
		}

		for _, i := range debug.GetProfileWriters() {
			w.Write([]byte(fmt.Sprintf(`
<li><a href="/debug/profile/%s/html">%s</a></li>`,
				url.QueryEscape(i.Name),
				html.EscapeString(i.Description))))
		}

		w.Write([]byte(`</body></html>`))
	}
}

func initDebugServer(config_obj *config_proto.Config) error {
	if *debug_flag {
		config_obj.DebugMode = true

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> debug server on <cyan>http://127.0.0.1:%v/debug/pprof", *debug_flag_port)

		http.HandleFunc("/debug/queries/", maybeRenderHTML(handleQueries))
		http.HandleFunc("/debug/profile/", maybeRenderHTML(
			handleProfile(config_obj)))
		http.HandleFunc("/debug/queries/running/",
			maybeRenderHTML(handleRunningQueries))
		http.HandleFunc("/", handleIndex(config_obj))

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
