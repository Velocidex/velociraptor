package authenticators

import (
	"net/http"
	"text/template"

	utils "www.velocidex.com/golang/velociraptor/api/utils"
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
		BasePath: utils.GetBasePath(config_obj),
		ErrState: json.MustMarshalString(velociraptor.ErrState{
			Type:           "Login",
			Username:       username,
			Authenticators: authenticators,
			BasePath:       utils.GetBasePath(config_obj),
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
		BasePath: utils.GetBasePath(config_obj),
		ErrState: json.MustMarshalString(velociraptor.ErrState{
			Type:           "Logoff",
			Username:       username,
			BasePath:       utils.Join(utils.GetBasePath(config_obj), "/"),
			Authenticators: []velociraptor.AuthenticatorInfo{},
		}),
	})
	if err != nil {
		//utils.Debug(err)
		w.WriteHeader(500)
	}
}
