package server

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/pprof"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"www.velocidex.com/golang/velociraptor/actions"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/golang"
	"www.velocidex.com/golang/vfilter"
)

// Dumps out the query log
func (self *debugMux) handleQueries(w http.ResponseWriter, r *http.Request) {
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
func (self *debugMux) handleRunningQueries(w http.ResponseWriter, r *http.Request) {
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

func (self *debugMux) renderHTML(w http.ResponseWriter, r *http.Request) {
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
		HtmlTemplate,
		html.EscapeString(profile_type),
		html.EscapeString(description))))
}

func (self *debugMux) HandleProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	profile_type, _ := url.QueryUnescape(path.Base(r.URL.Path))

	format := "jsonl"
	if profile_type == "json" {
		format = "json"
		profile_type, _ = url.QueryUnescape(path.Base(path.Dir(r.URL.Path)))
	}

	if profile_type == "profile" || profile_type == "all" {
		profile_type = ".+"
	} else {
		// Take the profile type as a literal string not a regex
		profile_type = regexp.QuoteMeta(profile_type)
	}

	builder := services.ScopeBuilder{
		Config:     self.config_obj,
		ACLManager: acl_managers.NullACLManager{},
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(self.config_obj)
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

type debugMux struct {
	config_obj *config_proto.Config
	base       string
}

func (self *debugMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	url_path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(url_path, "/"), "/")
	if len(parts) <= 1 {
		self.HandleIndex(w, r)
		return
	}

	// If the URL ends with html we render a html page that allows us
	// to access the raw data.
	if parts[len(parts)-1] == "html" {
		self.renderHTML(w, r)
		return
	}

	switch parts[1] {
	case "profile":
		self.HandleProfile(w, r)

	case "pprof":
		if len(parts) == 2 {
			pprof.Index(w, r)
			return
		}

		switch parts[2] {
		case "profile":
			pprof.Profile(w, r)
		case "symbol":
			pprof.Symbol(w, r)
		case "trace":
			pprof.Trace(w, r)
		default:
			pprof.Index(w, r)
		}

	case "queries":
		if len(parts) > 2 {
			self.handleRunningQueries(w, r)
			return
		}

		self.handleQueries(w, r)

	case "metrics":
		promhttp.Handler().ServeHTTP(w, r)

	default:
		self.HandleIndex(w, r)
	}
}

func (self *debugMux) HandleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	w.Write([]byte(fmt.Sprintf(`
<html><body>
<h1>Debug Server</h1>
<ul>
<li><a href="%s/debug/pprof/">Show built in Go Profiles</a></li>
<li><a href="%s/debug/queries/html">Show all queries</a></li>
<li><a href="%s/debug/queries/running/html">Show currently running queries</a></li>
<li><a href="%s/debug/profile/all/html">Show all profile items</a></li>
`, self.base, self.base, self.base, self.base)))

	w.Write([]byte(fmt.Sprintf(
		"<li><a href=\"%s/debug/metrics\">Metrics</a></li>\n",
		self.base)))

	for _, i := range debug.GetProfileWriters() {
		w.Write([]byte(fmt.Sprintf(`
<li><a href="%s/debug/profile/%s/html">%s</a></li>`,
			self.base,
			url.QueryEscape(i.Name),
			html.EscapeString(i.Description))))
	}

	w.Write([]byte(`</body></html>`))
}

func DebugMux(config_obj *config_proto.Config, base string) *debugMux {
	return &debugMux{
		config_obj: config_obj,
		base:       base,
	}
}
