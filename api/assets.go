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
	"www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/gui/velociraptor"
	gui_assets "www.velocidex.com/golang/velociraptor/gui/velociraptor"
	users "www.velocidex.com/golang/velociraptor/users"
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
		// It is possible that the binary was not built with the GUI
		// app. This is not a fatal error but it is not very useful :-).
		data = []byte(
			`<html><body>
  <h1>This binary was not build with GUI support!</h1>

  Search for building instructions on https://docs.velociraptor.app/
</body></html>`)
	}

	tmpl, err := template.New("").Parse(string(data))
	if err != nil {
		return nil, err
	}

	base := config_obj.GUI.BasePath

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userinfo := GetUserInfo(r.Context(), config_obj)

		// This should never happen!
		if userinfo.Name == "" {
			returnError(w, 401, "Unauthenticated access.")
			return
		}

		user_options, err := users.GetUserOptions(config_obj, userinfo.Name)
		if err != nil {
			// Options may not exist yet
			user_options = &proto.SetGUIOptionsRequest{}
		}

		args := velociraptor.HTMLtemplateArgs{
			Timestamp: time.Now().UTC().UnixNano() / 1000,
			CsrfToken: csrf.Token(r),
			BasePath:  base,
			Heading:   "Heading",
			UserTheme: user_options.Theme,
		}
		err = tmpl.Execute(w, args)
		if err != nil {
			w.WriteHeader(500)
		}
	}), nil
}
