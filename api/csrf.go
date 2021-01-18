package api

import (
	"crypto/sha256"
	"net/http"
	"os"

	"github.com/gorilla/csrf"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

// Wrap only a single handler with csrf protection.
func csrfProtect(config_obj *config_proto.Config,
	parent http.Handler) http.Handler {

	// We may need to disabled CSRF for benchmarking tests.
	disable_csrf, pres := os.LookupEnv("VELOCIRAPTOR_DISABLE_CSRF")
	if pres && disable_csrf == "1" {
		logger := logging.GetLogger(config_obj, &logging.GUIComponent)
		logger.Info("Disabling CSRF protection because environment VELOCIRAPTOR_DISABLE_CSRF is set")
		return parent
	}

	// Derive a CSRF key from the hash of the server's public key.
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(config_obj.Frontend.PrivateKey))
	token := hasher.Sum(nil)

	protectionFn := csrf.Protect(token, csrf.Path("/"), csrf.MaxAge(7*24*60*60))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protectionFn(parent).ServeHTTP(w, r)
	})
}
