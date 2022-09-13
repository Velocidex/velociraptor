package authenticators

import (
	"net/http"
	"text/template"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/gui/velociraptor"
	gui_assets "www.velocidex.com/golang/velociraptor/gui/velociraptor"
	"www.velocidex.com/golang/velociraptor/json"
)

func renderRejectionMessage(
	config_obj *config_proto.Config,
	w http.ResponseWriter, username string,
	authenticators []velociraptor.AuthenticatorInfo) {

	data, err := gui_assets.ReadFile("/index.html")
	if err != nil {
		//utils.Debug(err)
		w.WriteHeader(500)
		return
	}

	tmpl, err := template.New("").Parse(string(data))
	if err != nil {
		//utils.Debug(err)
		w.WriteHeader(500)
		return
	}

	err = tmpl.Execute(w, velociraptor.HTMLtemplateArgs{
		ErrState: json.MustMarshalString(velociraptor.ErrState{
			Type:           "Login",
			Username:       username,
			Authenticators: authenticators,
			BasePath:       getBasePath(config_obj),
		}),
	})
	if err != nil {
		//utils.Debug(err)
		w.WriteHeader(500)
	}
}

func renderLogoffMessage(
	config_obj *config_proto.Config,
	w http.ResponseWriter, username string) {
	data, err := gui_assets.ReadFile("/index.html")
	if err != nil {
		//utils.Debug(err)
		w.WriteHeader(500)
		return
	}

	tmpl, err := template.New("").Parse(string(data))
	if err != nil {
		//utils.Debug(err)
		w.WriteHeader(500)
		return
	}

	err = tmpl.Execute(w, velociraptor.HTMLtemplateArgs{
		ErrState: json.MustMarshalString(velociraptor.ErrState{
			Type:           "Logoff",
			Username:       username,
			BasePath:       getBasePath(config_obj),
			Authenticators: []velociraptor.AuthenticatorInfo{},
		}),
	})
	if err != nil {
		//utils.Debug(err)
		w.WriteHeader(500)
	}
}
