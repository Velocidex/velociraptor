/*
  An authenticator that uses client side certificates.

  WARNING: This authenticator is considered very experimental!!! There
  are serious security considerations when using this so ensure you
  understand all the ramifications before using it!

  This authenticator makes it possible to use distributed
  authentication - if the user has the client certificate they will be
  automatically authenticated! This is more risky than centralized
  authentication because the security depends on the certificates
  themselves.

  ## How to issue client certificates

  This authenticator uses the same certificates that are used in the
  Velociraptor api. You can use the `config api_client` command to
  generate new client certificates.

  velociraptor --config server.config.yaml config api_client --name Mike --pkcs12 mike.pkcs12 Mike.pem -v --password

  For convenience you can use the --pkcs12 flag to also save the
  certificates in .pkcs12 format which can be imported into the
  Windows trust store. It is recommended you use --password to armour
  the certificates.

  ## Configuring the server for client certificates

  The server's authenticator can be configured by replacing the Basic
  authenticator with the `Certs` authenticator under the GUI section.

  ```
  authenticator:
    type: Certs
    default_roles_for_unknown_user:
     - reader
     - administrator
  ```

  You can specify roles under `default_roles_for_unknown_user` which
  allows the server to automatically create user accounts with these
  roles when a client certificate is presented for an unknown
  users. Be careful with this setting as it might allow anyone with a
  valid signed certificate (even an API certificate) to elevate to
  administrator. It is recommended the API *not* be used when using
  this feature.

  ## How can I revoke a certificate?

  Currently certificates can not be revoked. Instead all the ACLs can
  be removed from the user account which mean that user has no
  access. You can not safely reuse the same user name once these
  permissions are removed.

  ## Caveats

  It is not possible for clients to present an TLS client certificate
  because they dont have one. Therefore the Frontend (the service
  connecting to clients) can not require client certificates. Since
  TLS requires client certificates *before* the HTTP headers it is
  currently impossible to require client certificates **only** for the
  GUI and not the frontend if they share the same port!!!

  This means that client certifacts do not work with using autocert
  (in that case both frontend and GUI share the same port due to
  limitations in the Let's Encrypt protocol).

  The server will refuse to start when the frontend is forced to use
  client certificates.

*/

package authenticators

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"

	"github.com/Velocidex/ordereddict"
	"github.com/gorilla/csrf"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	invalidCertError = errors.New("Invalid Client Certificate")
)

// Certificate based authenticator.
type CertAuthenticator struct {
	config_obj    *config_proto.Config
	x509_roots    *x509.CertPool
	default_roles []string
}

// Cert auth does not need any special handlers.
func (self *CertAuthenticator) AddHandlers(mux *api_utils.ServeMux) error {
	return nil
}

// It is not really possible to log off when using client certs
func (self *CertAuthenticator) AddLogoff(mux *api_utils.ServeMux) error {
	mux.Handle(api_utils.GetBasePath(self.config_obj, "/app/logoff.html"),
		IpFilter(self.config_obj,
			api_utils.HandlerFunc(nil,
				func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
					http.Error(w, "authorization failed", http.StatusUnauthorized)
					return
				})))

	return nil
}

func (self *CertAuthenticator) IsPasswordLess() bool {
	return true
}

func (self *CertAuthenticator) RequireClientCerts() bool {
	return true
}

func (self *CertAuthenticator) AuthRedirectTemplate() string {
	return ""
}

func (self *CertAuthenticator) getUserNameFromTLSCerts(r *http.Request) (string, error) {
	// We only trust certs issued by the Velociraptor CA.
	x509_opts := x509.VerifyOptions{
		CurrentTime: utils.GetTime().Now(),
		Roots:       self.x509_roots,
	}

	for _, cert := range r.TLS.PeerCertificates {
		_, err := cert.Verify(x509_opts)
		if err != nil {
			continue
		}
		return cert.Subject.CommonName, nil
	}
	return "", invalidCertError
}

func (self *CertAuthenticator) AuthenticateUserHandler(
	parent http.Handler,
	permission acls.ACL_PERMISSION,
) http.Handler {

	logger := GetLoggingHandler(self.config_obj)(parent)

	return api_utils.HandlerFunc(parent,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-CSRF-Token", csrf.Token(r))

			username, err := self.getUserNameFromTLSCerts(r)
			if err != nil {
				http.Error(w,
					fmt.Sprintf("authorization failed: Client Certificate is not valid: %v", err),
					http.StatusUnauthorized)
				return
			}

			users_manager := services.GetUserManager()
			user_record, err := users_manager.GetUser(r.Context(), username, username)
			if err != nil {
				if utils.IsNotFound(err) ||
					len(self.default_roles) == 0 {
					http.Error(w,
						fmt.Sprintf("authorization failed for %v: %v", username, err),
						http.StatusUnauthorized)
					return
				}

				// Create a new user role on the fly.
				policy := &acl_proto.ApiClientACL{
					Roles: self.default_roles,
				}
				err := services.LogAudit(r.Context(),
					self.config_obj, username, "Automatic User Creation",
					ordereddict.NewDict().
						Set("roles", self.default_roles).
						Set("remote", r.RemoteAddr))
				if err != nil {
					logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
					logger.Error("GetUser LogAudit: Automatic User Creation %v %v",
						username, r.RemoteAddr)
				}

				// Use the super user principal to actually add the
				// username so we have enough permissions.
				err = users_manager.AddUserToOrg(r.Context(), services.AddNewUser,
					utils.GetSuperuserName(self.config_obj), username,
					[]string{"root"}, policy)
				if err != nil {
					http.Error(w,
						fmt.Sprintf("authorization failed: automatic user creation: %v", err),
						http.StatusUnauthorized)
					return
				}

				user_record, err = users_manager.GetUser(r.Context(), username, username)
				if err != nil {
					http.Error(w,
						fmt.Sprintf("Failed creating user for %v: %v", username, err),
						http.StatusUnauthorized)
					return
				}
			}

			// Does the user have access to the specified org?
			err = CheckOrgAccess(self.config_obj, r, user_record, permission)
			if err != nil {
				err := services.LogAudit(r.Context(),
					self.config_obj, user_record.Name, "Unauthorized username",
					ordereddict.NewDict().
						Set("remote", r.RemoteAddr).
						Set("status", http.StatusUnauthorized))
				if err != nil {
					logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
					logger.Error("CheckOrgAccess LogAudit: Unauthorized username %v %v",
						user_record.Name, r.RemoteAddr)
				}

				http.Error(w,
					fmt.Sprintf("authorization failed: %v", err),
					http.StatusUnauthorized)
				return
			}

			// Checking is successful - user authorized. Here we
			// build a token to pass to the underlying GRPC
			// service with metadata about the user.
			user_info := &api_proto.VelociraptorUser{
				Name: user_record.Name,
			}

			// Must use json encoding because grpc can not handle
			// binary data in metadata.
			serialized, _ := json.Marshal(user_info)
			ctx := context.WithValue(
				r.Context(), constants.GRPC_USER_CONTEXT, string(serialized))

			// Need to call logging after auth so it can access
			// the USER value in the context.
			logger.ServeHTTP(w, r.WithContext(ctx))
		}).AddChild("GetLoggingHandler")
}
