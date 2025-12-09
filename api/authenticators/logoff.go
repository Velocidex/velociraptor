package authenticators

import (
	"net/http"
	"time"

	"github.com/Velocidex/ordereddict"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

func installLogoff(config_obj *config_proto.Config, mux *api_utils.ServeMux) {
	mux.Handle(api_utils.GetBasePath(config_obj, "/app/logoff.html"),
		IpFilter(config_obj,
			api_utils.HandlerFunc(nil,
				func(w http.ResponseWriter, r *http.Request) {
					params := r.URL.Query()
					old_username, ok := params["username"]
					username := ""
					if ok && len(old_username) == 1 {
						err := services.LogAudit(r.Context(),
							config_obj, old_username[0], "LogOff", ordereddict.NewDict())
						if err != nil {
							logger := logging.GetLogger(
								config_obj, &logging.FrontendComponent)
							logger.Error("LogAudit: LogOff %v", old_username[0])
						}
						username = old_username[0]
					}

					// Clear the cookie
					http.SetCookie(w, &http.Cookie{
						Name:     "VelociraptorAuth",
						Path:     api_utils.GetBaseDirectory(config_obj),
						Value:    "deleted",
						Secure:   true,
						HttpOnly: true,
						Expires:  time.Unix(0, 0),
					})

					renderLogoffMessage(config_obj, w, username)
				})))
}
