/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/gorilla/csrf"
	"github.com/lpar/gzipped"
	"www.velocidex.com/golang/velociraptor/api/proto"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/gui/velociraptor"
	gui_assets "www.velocidex.com/golang/velociraptor/gui/velociraptor"
	"www.velocidex.com/golang/velociraptor/services"
	vutils "www.velocidex.com/golang/velociraptor/utils"
)

func install_static_assets(
	ctx context.Context,
	config_obj *config_proto.Config, mux *api_utils.ServeMux) {
	base := utils.GetBasePath(config_obj)
	dir := utils.Join(base, "/app/")
	mux.Handle(dir, ipFilter(config_obj, api_utils.StripPrefix(
		dir, fixCSSURLs(config_obj,
			gzipped.FileServer(NewCachedFilesystem(ctx, gui_assets.NewHTTPFS()))))))

	mux.Handle("/favicon.png",
		http.RedirectHandler(utils.Join(base, "/favicon.ico"),
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
  <h1>This binary was not built with GUI support!</h1>

  Search for building instructions on https://docs.velociraptor.app/
</body></html>`)
	}

	tmpl, err := template.New("").Parse(string(data))
	if err != nil {
		return nil, err
	}

	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
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

// Vite hard compiles the css urls into the bundle so we can not move
// the base_path. This handler fixes this.
func fixCSSURLs(config_obj *config_proto.Config,
	parent http.Handler) http.Handler {

	if config_obj.GUI == nil || config_obj.GUI.BasePath == "" {
		return api_utils.HandlerFunc(parent, parent.ServeHTTP).
			AddChild("NewInterceptingResponseWriter")
	}

	return api_utils.HandlerFunc(parent,
		func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, ".css") {
				parent.ServeHTTP(w, r)
			} else {
				parent.ServeHTTP(
					NewInterceptingResponseWriter(config_obj, w, r), r)
			}
		}).AddChild("NewInterceptingResponseWriter")
}

type interceptingResponseWriter struct {
	http.ResponseWriter

	from, to string

	br_writer *brotli.Writer
}

// Replace base path in the CSS url properties.
func (self *interceptingResponseWriter) Write(buf []byte) (int, error) {
	new_buf := bytes.ReplaceAll(buf, []byte(self.from), []byte(self.to))
	// No compression
	if self.br_writer == nil {
		_, err := self.ResponseWriter.Write(new_buf)
		return len(buf), err
	}

	// Implement brotli compression
	_, err := self.br_writer.Write(new_buf)
	if err != nil {
		return 0, err
	}
	err = self.br_writer.Flush()
	return len(buf), err
}

func NewInterceptingResponseWriter(
	config_obj *config_proto.Config,
	w http.ResponseWriter, r *http.Request) http.ResponseWriter {

	// Try to do brotli compression if it is available.
	accept_encoding, pres := r.Header["Accept-Encoding"]
	if pres && len(accept_encoding) > 0 {
		parts := strings.Split(accept_encoding[0], ", ")
		if vutils.InString(parts, "br") {
			w.Header()["Content-Encoding"] = []string{"br"}

			return &interceptingResponseWriter{
				ResponseWriter: w,
				from:           "url(/app/assets/",
				to: fmt.Sprintf("url(%v/app/assets/",
					utils.GetBasePath(config_obj)),
				br_writer: brotli.NewWriter(w),
			}
		}
	}

	// Otherwise just pass through
	return &interceptingResponseWriter{
		ResponseWriter: w,
		from:           "url(/app/assets/",
		to: fmt.Sprintf("url(%v/app/assets/",
			utils.GetBasePath(config_obj)),
	}
}
