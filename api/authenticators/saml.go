package authenticators

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/crewjam/saml/samlsp"
	"github.com/gorilla/csrf"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/users"
)

var samlMiddleware *samlsp.Middleware

type SamlAuthenticator struct{}

func (self *SamlAuthenticator) IsPasswordLess() bool {
	return true
}

func (self *SamlAuthenticator) AddHandlers(config_obj *config_proto.Config, mux *http.ServeMux) error {
	auther := config_obj.GUI.Authenticator
	logger := logging.Manager.GetLogger(config_obj, &logging.GUIComponent)
	key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr([]byte(auther.SamlPrivateKey))
	if err != nil {
		return err
	}

	cert, err := crypto_utils.ParseX509CertFromPemStr([]byte(auther.SamlCertificate))
	if err != nil {
		return err
	}

	idpMetadataURL, err := url.Parse(auther.SamlIdpMetadataUrl)
	if err != nil {
		return err
	}

	rootURL, err := url.Parse(auther.SamlRootUrl)
	if err != nil {
		return err
	}
	samlMiddleware, err = samlsp.New(samlsp.Options{
		IDPMetadataURL: idpMetadataURL,
		URL:            *rootURL,
		Key:            key,
		Certificate:    cert,
	})
	if err != nil {
		return err
	}
	mux.Handle("/saml/", samlMiddleware)
	logger.Info("Authentication via SAML enabled")
	return nil
}

func (self *SamlAuthenticator) AuthenticateUserHandler(
	config_obj *config_proto.Config,
	parent http.Handler) http.Handler {

	reject_handler := samlMiddleware.RequireAccount(parent)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", csrf.Token(r))

		session, err := samlMiddleware.Session.GetSession(r)
		if session == nil {
			reject_handler.ServeHTTP(w, r)
			return
		}

		username := samlsp.AttributeFromContext(r.Context(), userAttr(config_obj))
		user_record, err := users.GetUser(config_obj, username)

		perm, err2 := acls.CheckAccess(config_obj, username, acls.READ_RESULTS)
		if err != nil || !perm || err2 != nil ||
			user_record.Locked || user_record.Name != username {
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
			r.Context(), constants.GRPC_USER_CONTEXT,
			string(serialized))
		GetLoggingHandler(config_obj)(parent).ServeHTTP(
			w, r.WithContext(ctx))
		return
	})
}

func userAttr(config_obj *config_proto.Config) string {
	auther := config_obj.GUI.Authenticator
	if auther.SamlUserAttribute == "" {
		return "name"
	}
	return auther.SamlUserAttribute
}
