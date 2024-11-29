package authenticators

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/crewjam/saml/samlsp"
	"github.com/gorilla/csrf"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var samlMiddleware *samlsp.Middleware

type SamlAuthenticator struct {
	config_obj     *config_proto.Config
	user_attribute string
	authenticator  *config_proto.Authenticator
	user_roles     []string
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

func (self *SamlAuthenticator) AddHandlers(mux *api_utils.ServeMux) error {
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

	opts := samlsp.Options{
		IDPMetadata: idpMetadata,
		URL:         *rootURL,
		Key:         key,
		Certificate: cert,
	}
	samlMiddleware, err = samlsp.New(opts)
	if err != nil {
		return err
	}

	expiry_min := self.authenticator.DefaultSessionExpiryMin
	if expiry_min == 0 {
		expiry_min = 60 * 24 // 1 Day by default
	}
	maxAge := time.Minute * time.Duration(expiry_min)
	jwtSessionCodec := samlsp.DefaultSessionCodec(opts)
	jwtSessionCodec.MaxAge = maxAge
	cookieSessionProvider := samlsp.DefaultSessionProvider(opts)
	cookieSessionProvider.MaxAge = maxAge
	cookieSessionProvider.Codec = jwtSessionCodec
	samlMiddleware.Session = cookieSessionProvider

	mux.Handle(api_utils.GetBasePath(self.config_obj, "/saml/"),
		IpFilter(self.config_obj, samlMiddleware))
	logger.Info("Authentication via SAML enabled")
	return nil
}

func (self *SamlAuthenticator) AddLogoff(mux *api_utils.ServeMux) error {
	installLogoff(self.config_obj, mux)
	return nil
}

func (self *SamlAuthenticator) AuthenticateUserHandler(
	parent http.Handler) http.Handler {

	reject_handler := samlMiddleware.RequireAccount(parent)

	logger := GetLoggingHandler(self.config_obj)(parent)

	return api_utils.HandlerFunc(parent,
		func(w http.ResponseWriter, r *http.Request) {
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
			user_record, err := users.GetUser(r.Context(), username, username)
			if err != nil {
				if !errors.Is(err, utils.NotFoundError) {
					http.Error(w,
						fmt.Sprintf("authorization failed: %v", err),
						http.StatusUnauthorized)

					services.LogAudit(r.Context(),
						self.config_obj, username, "Authorization failed",
						ordereddict.NewDict().
							Set("error", err).
							Set("username", username).
							Set("roles", self.user_roles).
							Set("remote", r.RemoteAddr))
					return
				}

				if len(self.user_roles) == 0 {
					http.Error(w,
						"authorization failed: no saml user roles assigned",
						http.StatusUnauthorized)

					services.LogAudit(r.Context(),
						self.config_obj, username, "Authorization failed: no saml user roles assigned",
						ordereddict.NewDict().
							Set("username", username).
							Set("roles", self.user_roles).
							Set("remote", r.RemoteAddr))
					return
				}

				// Create a new user role on the fly.
				policy := &acl_proto.ApiClientACL{
					Roles: self.user_roles,
				}
				services.LogAudit(r.Context(),
					self.config_obj, username, "Automatic User Creation",
					ordereddict.NewDict().
						Set("username", username).
						Set("roles", self.user_roles).
						Set("remote", r.RemoteAddr))

				// Use the super user principal to actually add the
				// username so we have enough permissions.
				err = users.AddUserToOrg(r.Context(), services.AddNewUser,
					constants.PinnedServerName, username,
					[]string{"root"}, policy)
				if err != nil {
					http.Error(w,
						fmt.Sprintf("authorization failed: automatic user creation: %v", err),
						http.StatusUnauthorized)
					return
				}

				user_record, err = users.GetUser(r.Context(), username, username)
				if err != nil {
					http.Error(w,
						fmt.Sprintf("Failed creating user for %v: %v", username, err),
						http.StatusUnauthorized)
					return
				}
			}

			// Does the user have access to the specified org?
			err = CheckOrgAccess(self.config_obj, r, user_record)
			if err != nil {
				services.LogAudit(r.Context(),
					self.config_obj, username, "authorization failed: user not registered and no saml_user_roles set",
					ordereddict.NewDict().
						Set("username", username).
						Set("roles", self.user_roles).
						Set("remote", r.RemoteAddr).
						Set("status", http.StatusUnauthorized))

				http.Error(w,
					fmt.Sprintf("authorization failed: user not registered - contact your system administrator: %v", err),
					http.StatusUnauthorized)
				return
			}

			user_info := &api_proto.VelociraptorUser{
				Name: user_record.Name,
			}

			serialized, _ := json.Marshal(user_info)
			ctx := context.WithValue(
				r.Context(), constants.GRPC_USER_CONTEXT,
				string(serialized))
			logger.ServeHTTP(w, r.WithContext(ctx))
		}).AddChild("GetLoggingHandler")
}

func NewSamlAuthenticator(
	config_obj *config_proto.Config,
	auther *config_proto.Authenticator) (*SamlAuthenticator, error) {
	result := &SamlAuthenticator{
		config_obj:     config_obj,
		user_attribute: "name",
		authenticator:  auther,
		user_roles:     auther.SamlUserRoles,
	}

	if auther.SamlUserAttribute != "" {
		result.user_attribute = auther.SamlUserAttribute
	}
	return result, nil
}
