package api

import (
	"crypto/sha256"
	"net/http"
	"os"

	"github.com/gorilla/csrf"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

// Wrap only a single handler with csrf protection.
func csrfProtect(config_obj *config_proto.Config,
	parent http.Handler) http.Handler {

	// We may need to disabled CSRF for benchmarking tests.
	disable_csrf, pres := os.LookupEnv("VELOCIRAPTOR_DISABLE_CSRF")
	if pres && disable_csrf == "1" {
		logger := logging.GetLogger(config_obj, &logging.GUIComponent)
		logger.Info("Disabling CSRF protection because environment VELOCIRAPTOR_DISABLE_CSRF is set")
		return api_utils.HandlerFunc(parent, parent.ServeHTTP)
	}

	// Derive a CSRF key from the hash of the server's public key.
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(config_obj.Frontend.PrivateKey))
	token := hasher.Sum(nil)

	trusted_origins := append([]string{}, config_obj.GUI.TrustedOrigins...)
	frontend_service, err := services.GetFrontendManager(config_obj)
	if err == nil {
		public_url, err := frontend_service.GetPublicUrl(config_obj)
		if err == nil && public_url.Host != "" {
			trusted_origins = append(trusted_origins, public_url.Host)
		}
	}

	protectionFn := csrf.Protect(
		token,
		csrf.Path("/"),
		csrf.SameSite(csrf.SameSiteStrictMode),
		csrf.TrustedOrigins(trusted_origins),
		csrf.MaxAge(7*24*60*60))

	return api_utils.HandlerFunc(parent,
		func(w http.ResponseWriter, r *http.Request) {
			protectionFn(parent).ServeHTTP(w, r)
		})
}
