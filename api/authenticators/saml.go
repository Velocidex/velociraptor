package authenticators

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Velocidex/ordereddict"
	"github.com/crewjam/saml/samlsp"
	"github.com/gorilla/csrf"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

var samlMiddleware *samlsp.Middleware

type SamlAuthenticator struct {
	config_obj     *config_proto.Config
	user_attribute string
	authenticator  *config_proto.Authenticator
}

func (self *SamlAuthenticator) IsPasswordLess() bool {
	return true
}

func (self *SamlAuthenticator) RequireClientCerts() bool {
	return false
}

func (self *SamlAuthenticator) AuthRedirectTemplate() string {
	return self.authenticator.AuthRedirectTemplate
}

func (self *SamlAuthenticator) AddHandlers(mux *http.ServeMux) error {

	logger := logging.Manager.GetLogger(self.config_obj, &logging.GUIComponent)
	key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr([]byte(
		self.authenticator.SamlPrivateKey))
	if err != nil {
		return err
	}

	cert, err := crypto_utils.ParseX509CertFromPemStr([]byte(
		self.authenticator.SamlCertificate))
	if err != nil {
		return err
	}

	idpMetadataURL, err := url.Parse(self.authenticator.SamlIdpMetadataUrl)
	if err != nil {
		return err
	}

	rootURL, err := url.Parse(self.authenticator.SamlRootUrl)
	if err != nil {
		return err
	}

	idpMetadata, err := samlsp.FetchMetadata(
		context.Background(),
		http.DefaultClient,
		*idpMetadataURL)
	if err != nil {
		return err
	}

	samlMiddleware, err = samlsp.New(samlsp.Options{
		IDPMetadata: idpMetadata,
		URL:         *rootURL,
		Key:         key,
		Certificate: cert,
	})
	if err != nil {
		return err
	}
	mux.Handle("/saml/", samlMiddleware)
	logger.Info("Authentication via SAML enabled")
	return nil
}

func (self *SamlAuthenticator) AddLogoff(mux *http.ServeMux) error {
	installLogoff(self.config_obj, mux)
	return nil
}

func (self *SamlAuthenticator) AuthenticateUserHandler(
	parent http.Handler) http.Handler {

	reject_handler := samlMiddleware.RequireAccount(parent)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", csrf.Token(r))

		session, err := samlMiddleware.Session.GetSession(r)
		if session == nil {
			reject_handler.ServeHTTP(w, r)
			return
		}

		sa, ok := session.(samlsp.SessionWithAttributes)
		if !ok {
			reject_handler.ServeHTTP(w, r)
			return
		}

		username := sa.GetAttributes().Get(self.user_attribute)
		users := services.GetUserManager()
		user_record, err := users.GetUser(r.Context(), username)
		if err == nil && user_record.Name == username {
			// Does the user have access to the specified org?
			err = CheckOrgAccess(r, user_record)
		}

		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)

			fmt.Fprintf(w, `
<html><body>
Authorization failed. You are not registered on this system as %v.
Contact your system administrator to get an account, then try again.
</body></html>
`, username)

			services.LogAudit(r.Context(),
				self.config_obj, username, "User rejected by GUI",
				ordereddict.NewDict().
					Set("remote", r.RemoteAddr).
					Set("method", r.Method).
					Set("error", err.Error()))

			return
		}

		user_info := &api_proto.VelociraptorUser{
			Name: username,
		}

		serialized, _ := json.Marshal(user_info)
		ctx := context.WithValue(
			r.Context(), constants.GRPC_USER_CONTEXT,
			string(serialized))
		GetLoggingHandler(self.config_obj)(parent).ServeHTTP(
			w, r.WithContext(ctx))
		return
	})
}

func NewSamlAuthenticator(
	config_obj *config_proto.Config,
	auther *config_proto.Authenticator) (*SamlAuthenticator, error) {
	result := &SamlAuthenticator{
		config_obj:     config_obj,
		user_attribute: "name",
		authenticator:  auther,
	}

	if auther.SamlUserAttribute != "" {
		result.user_attribute = auther.SamlUserAttribute
	}
	return result, nil
}
