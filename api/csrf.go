package api

import (
	"crypto/sha256"
	"net/http"

	"github.com/gorilla/csrf"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Wrap only a single handler with csrf protection.
func csrfProtect(config_obj *config_proto.Config,
	parent http.Handler) http.Handler {

	// Derive a CSRF key from the hash of the server's public key.
	hasher := sha256.New()
	hasher.Write([]byte(config_obj.Frontend.PrivateKey))
	token := hasher.Sum(nil)

	protectionFn := csrf.Protect(token, csrf.Path("/"))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protectionFn(parent).ServeHTTP(w, r)
	})
}
