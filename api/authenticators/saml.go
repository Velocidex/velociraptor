package authenticators

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/crewjam/saml/samlsp"
	"github.com/gorilla/csrf"
	"www.velocidex.com/golang/velociraptor/acls"
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
	logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)
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
		IDPMetadata:       idpMetadata,
		URL:               *rootURL,
		Key:               key,
		Certificate:       cert,
		AllowIDPInitiated: self.authenticator.SamlAllowIdpInitiated,
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
	parent http.Handler,
	permission acls.ACL_PERMISSION,
) http.Handler {

	reject_handler := samlMiddleware.RequireAccount(parent)

	logger := GetLoggingHandler(self.config_obj)(parent)

	return api_utils.HandlerFunc(parent,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-CSRF-Token", csrf.Token(r))

			session, err := samlMiddleware.Session.GetSession(r)
			if session == nil || err != nil {
				reject_handler.ServeHTTP(w, r)
				return
			}

			sa, ok := session.(samlsp.SessionWithAttributes)
			if !ok {
				reject_handler.ServeHTTP(w, r)
				return
			}

			username := sa.GetAttributes().Get(self.user_attribute)

			user_record, err := self.MaybeCreateUser(r.Context(), username, r.RemoteAddr)
			if err != nil {
				err := services.LogAudit(r.Context(),
					self.config_obj, username, "Authorization Failed",
					ordereddict.NewDict().
						Set("error", err).
						Set("username", username).
						Set("roles", self.user_roles).
						Set("remote", r.RemoteAddr))
				if err != nil {
					logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)
					logger.Error("<red>Authorization failed</> %v %v %v",
						username, err, r.RemoteAddr)
				}

				http.Error(w,
					fmt.Sprintf("authorization failed: %v", err),
					http.StatusUnauthorized)
				return
			}
			err = self.MaybeAssignRoles(r.Context(), username)
			if err != nil {
				err := services.LogAudit(r.Context(),
					self.config_obj, username, "Role Assignment Failed",
					ordereddict.NewDict().
						Set("username", username).
						Set("roles", self.user_roles).
						Set("remote", r.RemoteAddr))
				if err != nil {
					logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)
					logger.Error("<red>Role Assignment Failed</> %v %v",
						username, r.RemoteAddr)
				}

				http.Error(w,
					fmt.Sprintf("authorization failed: role assignment failed: %v", err),
					http.StatusUnauthorized)
				return
			}

			// Does the user have access to the specified org?
			err = CheckOrgAccess(self.config_obj, r, user_record, permission)
			if err != nil {
				err := services.LogAudit(r.Context(),
					self.config_obj, username, "authorization failed: user not registered and no saml_user_roles set",
					ordereddict.NewDict().
						Set("username", username).
						Set("roles", self.user_roles).
						Set("remote", r.RemoteAddr).
						Set("status", http.StatusUnauthorized))
				if err != nil {
					logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)
					logger.Error("<red>no saml_user_roles set</> %v %v",
						username, r.RemoteAddr)
				}

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

func (self *SamlAuthenticator) MaybeCreateUser(ctx context.Context, username string, remote string) (*api_proto.VelociraptorUser, error) {
	user_manager := services.GetUserManager()
	user_record, err := user_manager.GetUser(ctx, username, username)

	if utils.IsNotFound(err) {
		// we only create users if the "user_roles" option is set
		if len(self.user_roles) == 0 {
			_ = services.LogAudit(ctx,
				self.config_obj, username, "Authorization failed: no saml user roles assigned",
				ordereddict.NewDict().
					Set("username", username).
					Set("roles", self.user_roles).
					Set("remote", remote))
			return nil, err
		}

		user_record := &api_proto.VelociraptorUser{
			Name: username,
		}
		err = services.LogAudit(ctx, self.config_obj, username,
			"Create User From SAML",
			ordereddict.NewDict().Set("username", username).Set("roles", self.user_roles).Set("remote", remote))
		if err != nil {
			return nil, err
		}

		err = user_manager.SetUser(ctx, user_record)
		if err != nil {
			return nil, err
		}

		return user_record, nil
	} else {
		return user_record, err
	}
}

func (self *SamlAuthenticator) MaybeAssignRoles(
	ctx context.Context,
	username string,
) error {
	if len(self.user_roles) == 0 {
		return nil
	}

	// Usually roles are set per org but setting roles through the
	// SAML IDP will grant the roles on all orgs.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	for _, org := range org_manager.ListOrgs() {
		org_config_obj, err := org_manager.GetOrgConfig(org.Id)
		if err != nil {
			continue
		}

		// Get the user's ACL policy in that org
		existing_acls, err := services.GetPolicy(org_config_obj, username)
		if err != nil {
			// If a user does not exist this will fail to get their
			// policy so start with a fresh policy.
			existing_acls = &acl_proto.ApiClientACL{}
		}

		new_roles := append([]string{}, existing_acls.Roles...)
		// Add new roles
		for _, role := range self.user_roles {
			if !utils.InString(new_roles, role) {
				new_roles = append(new_roles, role)
			}
		}

		// Only set the roles if we need to
		if len(new_roles) > len(existing_acls.Roles) {
			err = services.LogAudit(ctx, self.config_obj, username,
				"Grant User Role From SAML",
				ordereddict.NewDict().
					Set("Roles", new_roles).
					Set("OrgId", org.Id))
			if err != nil {
				continue
			}
			err = services.GrantRoles(org_config_obj, username, new_roles)
			if err != nil {
				return err
			}
		}
	}
	return nil
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
