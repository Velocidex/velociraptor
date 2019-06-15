// +build !release

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

// Build during development (non release) - serve static assets
// directly from filesystem. We assume we are run from the top level
// directory using go run. e.g.:

// go run bin/*.go frontend test_data/server.config.yaml
package api

import (
	"html/template"
	"net/http"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

func install_static_assets(config_obj *api_proto.Config, mux *http.ServeMux) {
	logging.GetLogger(config_obj, &logging.FrontendComponent).
		Info("GUI will serve files from directory gui/static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(
		http.Dir("gui/static"))))
	mux.Handle("/favicon.ico",
		http.RedirectHandler("/static/images/favicon.ico",
			http.StatusMovedPermanently))
}

func GetTemplateHandler(config_obj *api_proto.Config,
	template_name string) (http.Handler, error) {
	tmpl, err := template.ParseFiles("gui" + template_name)
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		args := _templateArgs{
			Timestamp: time.Now().UTC().UnixNano() / 1000,
			Heading:   "Heading",
		}
		err := tmpl.Execute(w, args)
		if err != nil {
			w.WriteHeader(500)
		}
	}), nil
}
