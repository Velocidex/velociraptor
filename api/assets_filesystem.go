// +build !release

// Build during development (non release) - serve static assets
// directly from filesystem. We assume we are run from the top level
// directory using go run. e.g.:

// go run bin/*.go frontend test_data/server.config.yaml
package api

import (
	"html/template"
	"net/http"
	"time"
	config "www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/logging"
)

func install_mux(config_obj *config.Config, mux *http.ServeMux) {
	logging.NewLogger(config_obj).Info("GUI will serve files from directory gui/static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(
		http.Dir("gui/static"))))
}

func GetTemplateHandler(config_obj *config.Config,
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
