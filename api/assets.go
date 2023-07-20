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

// Build during release - embed all static assets in the binary.
package api

import (
	"html/template"
	"net/http"
	"time"

	"github.com/gorilla/csrf"
	"github.com/lpar/gzipped"
	"www.velocidex.com/golang/velociraptor/api/proto"
	utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/gui/velociraptor"
	gui_assets "www.velocidex.com/golang/velociraptor/gui/velociraptor"
	"www.velocidex.com/golang/velociraptor/services"
)

func install_static_assets(config_obj *config_proto.Config, mux *http.ServeMux) {
	base := utils.GetBasePath(config_obj)
	dir := utils.Join(base, "/app/")
	mux.Handle(dir, ipFilter(config_obj, http.StripPrefix(
		dir, gzipped.FileServer(NewCachedFilesystem(gui_assets.HTTP)))))

	mux.Handle("/favicon.png",
		http.RedirectHandler(utils.Join(base, "/favicon.ico"),
			http.StatusMovedPermanently))
}

func GetTemplateHandler(
	config_obj *config_proto.Config, template_name string) (http.Handler, error) {
	gui_assets.Init()

	data, err := gui_assets.ReadFile(template_name)
	if err != nil {
		// It is possible that the binary was not built with the GUI
		// app. This is not a fatal error but it is not very useful :-).
		data = []byte(
			`<html><body>
  <h1>This binary was not built with GUI support!</h1>

  Search for building instructions on https://docs.velociraptor.app/
</body></html>`)
	}

	tmpl, err := template.New("").Parse(string(data))
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userinfo := GetUserInfo(r.Context(), config_obj)

		// This should never happen!
		if userinfo.Name == "" {
			returnError(w, 401, "Unauthenticated access.")
			return
		}

		users := services.GetUserManager()
		user_options, err := users.GetUserOptions(r.Context(), userinfo.Name)
		if err != nil {
			// Options may not exist yet
			user_options = &proto.SetGUIOptionsRequest{}
		}

		args := velociraptor.HTMLtemplateArgs{
			Timestamp: time.Now().UTC().UnixNano() / 1000,
			CsrfToken: csrf.Token(r),
			BasePath:  utils.GetBasePath(config_obj),
			Heading:   "Heading",
			UserTheme: user_options.Theme,
			OrgId:     user_options.Org,
		}
		err = tmpl.Execute(w, args)
		if err != nil {
			w.WriteHeader(500)
		}
	}), nil
}
