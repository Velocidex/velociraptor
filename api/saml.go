package api

import (
	"context"
	"encoding/json"
	"fmt"
	_ "fmt"
	"net/http"
	"net/url"

	"github.com/crewjam/saml/samlsp"
	"github.com/sirupsen/logrus"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/users"
)

var samlMiddleware *samlsp.Middleware

func MaybeAddSAMLHandlers(config_obj *config_proto.Config, mux *http.ServeMux) error {
	if config.SAMLEnabled(config_obj) {
		key, err := crypto.ParseRsaPrivateKeyFromPemStr([]byte(config_obj.GUI.SamlPrivateKey))
		if err != nil {
			return err
		}

		cert, err := crypto.ParseX509CertFromPemStr([]byte(config_obj.GUI.SamlCertificate))
		if err != nil {
			return err
		}

		idpMetadataURL, err := url.Parse(config_obj.GUI.SamlIdpMetadataUrl)
		if err != nil {
			return err
		}

		rootURL, err := url.Parse(config_obj.GUI.SamlRootUrl)
		if err != nil {
			return err
		}

		samlMiddleware, _ = samlsp.New(samlsp.Options{
			IDPMetadataURL: idpMetadataURL,
			URL:            *rootURL,
			Key:            key,
			Certificate:    cert,
		})
		mux.Handle("/saml/", samlMiddleware)
	}

	return nil
}

func userAttr(config_obj *config_proto.Config) string {
	if config_obj.GUI.SamlUserAttribute == "" {
		return "name"
	}
	return config_obj.GUI.SamlUserAttribute
}

func authenticateSAML(config_obj *config_proto.Config, parent http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := samlMiddleware.GetAuthorizationToken(r); token != nil {

			username := token.Attributes.Get(userAttr(config_obj))
			user_record, err := users.GetUser(config_obj, username)
			if err != nil || user_record.Locked || user_record.Name != username {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)

				fmt.Fprintf(w, `
<html><body>
Authorization failed. You are not registered on this system as %v.
Contact your system administrator to get an account, then try again.
</body></html>
`, username)

				logging.GetLogger(config_obj, &logging.Audit).
					WithFields(logrus.Fields{
						"user":   username,
						"remote": r.RemoteAddr,
						"method": r.Method,
					}).Error("User rejected by GUI")

				return
			}

			user_info := &api_proto.VelociraptorUser{
				Name: username,
			}

			serialized, _ := json.Marshal(user_info)
			ctx := context.WithValue(
				r.Context(), "USER", string(serialized))
			GetLoggingHandler(config_obj)(parent).ServeHTTP(
				w, r.WithContext(ctx))
			return
		}
		samlMiddleware.RequireAccountHandler(w, r)
	})
}
