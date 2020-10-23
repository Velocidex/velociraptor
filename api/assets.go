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

// Build during release - embed all static assets in the binary.
package api

import (
	"html/template"
	"net/http"
	"time"

	"github.com/gorilla/csrf"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	gui_assets "www.velocidex.com/golang/velociraptor/gui/velociraptor"
	"www.velocidex.com/golang/velociraptor/utils"
)

func install_static_assets(config_obj *config_proto.Config, mux *http.ServeMux) {
	base := ""
	if config_obj.GUI != nil {
		base = config_obj.GUI.BasePath
	}
	dir := base + "/app/"
	mux.Handle(dir, http.StripPrefix(dir, http.FileServer(gui_assets.HTTP)))
	mux.Handle("/favicon.png",
		http.RedirectHandler(base+"/static/images/favicon.ico",
			http.StatusMovedPermanently))
}

func GetTemplateHandler(
	config_obj *config_proto.Config, template_name string) (http.Handler, error) {
	data, err := gui_assets.ReadFile(template_name)
	if err != nil {
		utils.Debug(err)
		return nil, err
	}

	tmpl, err := template.New("").Parse(string(data))
	if err != nil {
		return nil, err
	}

	base := config_obj.GUI.BasePath

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		args := _templateArgs{
			Timestamp: time.Now().UTC().UnixNano() / 1000,
			CsrfToken: csrf.Token(r),
			BasePath:  base,
			Heading:   "Heading",
		}
		err := tmpl.Execute(w, args)
		if err != nil {
			w.WriteHeader(500)
		}
	}), nil
}
