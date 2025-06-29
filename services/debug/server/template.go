package server

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path"

	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	HtmlTemplate = `
<html>
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
    <link rel="stylesheet" href="https://cdn.datatables.net/1.13.7/css/jquery.dataTables.min.css" crossorigin="anonymous">
    <script src="https://code.jquery.com/jquery-3.7.0.js"></script>
    <script src="https://cdn.datatables.net/1.13.7/js/jquery.dataTables.min.js"></script>
    <style>
      li.object { list-style: none; }
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
	indexHeader = `
<html>
  <head>
   <style>
    .cat-name { color: red; font-size: large; display: inline-block; }
    ul.categories { margin-top: 5px; margin-bottom: 5px; }
    .category { font-size: large; font-weight: bold; margin-top: 20px; }
   </style>
  </head>
<body>
<h1>Velociraptor Debug Server</h1>
<div class="category">Internal</div>
<ul class="categories">
  <li><a href="%s/debug/metrics">Metrics</a></li>
  <li><a href="%s/debug/pprof/">Show built in Go Profiles</a></li>
</ul>
`
)

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

	_, _ = w.Write([]byte(fmt.Sprintf(
		HtmlTemplate,
		html.EscapeString(profile_type),
		html.EscapeString(description))))
}

func (self *debugMux) renderCategory(node *debug.CategoryTreeNode) string {
	res := ""

	for _, k := range utils.Sort(node.SubCategories) {
		subc := node.SubCategories[k]

		name := subc.Path[len(subc.Path)-1]

		res += fmt.Sprintf("<div class=\"category\">%v</div>\n<ul class=\"categories\">",
			html.EscapeString(name))

		for _, k := range utils.Sort(subc.Profiles) {
			p := subc.Profiles[k]

			res += fmt.Sprintf(`
<li><span class="cat-name">%s</span>: <a href="%s/debug/profile/%s/html">%s</a></li>`,
				html.EscapeString(p.Name),
				self.base,
				url.QueryEscape(p.Name),
				html.EscapeString(p.Description))
		}
		res += self.renderCategory(subc)
		res += "</ul>\n"
	}

	return res
}

func (self *debugMux) HandleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	_, _ = w.Write([]byte(fmt.Sprintf(indexHeader, self.base, self.base)))
	categories := debug.GetProfileTree()
	_, _ = w.Write([]byte(self.renderCategory(categories)))
	_, _ = w.Write([]byte(`</body></html>`))
}
