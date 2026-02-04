package authenticators

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Velocidex/ordereddict"
	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/csrf"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/gui/velociraptor"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

var (
	reauthError = errors.New(`Authentication cookie not found, invalid or expired.
You probably need to re-authenticate in a new tab or refresh this page.`)
)

// Middleware function to enforce the request is authenticated.
//  1. Extract the claims from the JWT cookie
//  2. Make sure the user has the required permission on the specified org.
//  3. If user is authorized we pass a token to the gRPC gateway so it
//     can become available inside the API server. This way the HTTP
//     handler can identify the user, and pass that fact to the API
//     backend without needing to re-auth the user again.
func authenticateUserHandle(
	config_obj *config_proto.Config,
	permission acls.ACL_PERMISSION,
	reject_cb func(w http.ResponseWriter, r *http.Request,
		err error, username string),
	parent http.Handler) http.Handler {

	logger := GetLoggingHandler(config_obj)(parent)

	return api_utils.HandlerFunc(parent,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-CSRF-Token", csrf.Token(r))

			claims, err := getDetailsFromCookie(config_obj, r)
			if err != nil {
				reject_cb(w, r, err, claims.Username)
				return
			}

			username := claims.Username

			// Now check if the user is allowed to log in.
			users := services.GetUserManager()
			user_record, err := users.GetUser(r.Context(), username, username)
			if err != nil {
				reject_cb(w, r, fmt.Errorf("Invalid user: %v", err), username)
				return
			}

			// Does the user have access to the specified org?
			err = CheckOrgAccess(config_obj, r, user_record, permission)
			if err != nil {
				reject_cb(w, r, fmt.Errorf("Insufficient permissions: %v", err), user_record.Name)
				return
			}

			// Checking is successful - user authorized. Here we
			// build a token to pass to the underlying GRPC
			// service with metadata about the user.
			user_info := &api_proto.VelociraptorUser{
				Name: user_record.Name,
			}

			// NOTE: This context is NOT the same context that is received
			// by the API handlers. This context sits on the incoming side
			// of the GRPC gateway. We stuff our data into the
			// GRPC_USER_CONTEXT of the context and the code will convert
			// this value into a GRPC metadata.

			// Must use json encoding because grpc can not handle
			// binary data in metadata.
			serialized, _ := json.Marshal(user_info)
			ctx := context.WithValue(
				r.Context(), constants.GRPC_USER_CONTEXT, string(serialized))

			// Need to call logging after auth so it can access
			// the contextKeyUser value in the context.
			logger.ServeHTTP(w, r.WithContext(ctx))
		}).AddChild("GetLoggingHandler")
}

// Reject the user with a message and also add to the audit log.
func reject_with_username(
	config_obj *config_proto.Config,
	w http.ResponseWriter, r *http.Request,
	err error, username, login_url, provider, avatar string) {

	// Log failed login to the audit log only if there is an actual
	// user. First redirect will have username blank.
	if username != "" {
		err := services.LogAudit(r.Context(),
			config_obj, username, "User rejected by GUI",
			ordereddict.NewDict().
				Set("remote", r.RemoteAddr).
				Set("method", r.Method).
				Set("url", r.URL.String()).
				Set("err", err.Error()))
		if err != nil {
			logger := logging.GetLogger(
				config_obj, &logging.FrontendComponent)
			logger.Error("LogAudit: User rejected by GUI %v %v",
				username, r.RemoteAddr)
		}

	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)

	renderRejectionMessage(config_obj,
		r, w, err, username, []velociraptor.AuthenticatorInfo{
			{
				LoginURL:       api_utils.PublicURL(config_obj, login_url),
				ProviderAvatar: avatar,
				ProviderName:   provider,
			},
		})
}

// Create a new JWT cookie embedding the claims in it.
// The JWT is signed with the server's private key so we can verify it
// easily and it can not be modified.
func getSignedJWTTokenCookie(
	config_obj *config_proto.Config,
	authenticator *config_proto.Authenticator,
	claims *Claims, r *http.Request) (*http.Cookie, error) {
	if config_obj.Frontend == nil {
		return nil, errors.New("config has no Frontend")
	}

	expiry_min := authenticator.DefaultSessionExpiryMin
	if expiry_min == 0 {
		expiry_min = 60 * 24 // 1 Day by default
	}

	// We force expiry in the JWT **as well** as the session
	// cookie. The JWT expiry is most important as the browser can
	// replay session cookies past expiry.
	expiry := utils.GetTime().Now().Add(time.Minute * time.Duration(expiry_min))

	// Enforce the JWT to expire
	claims.Expires = float64(expiry.Unix())

	// Make a JWT and sign it.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	if authenticator.OidcDebug {
		logging.GetLogger(config_obj, &logging.GUIComponent).
			Debug("getSignedJWTTokenCookie: Creating JWT with claims: %#v", claims)
	}

	tokenString, err := token.SignedString([]byte(config_obj.Frontend.PrivateKey))
	if err != nil {
		return nil, err
	}

	// Log a successful login.
	err = services.LogAudit(r.Context(),
		config_obj, claims.Username, "Login",
		ordereddict.NewDict().
			Set("remote", r.RemoteAddr).
			Set("authenticator", authenticator.Type).
			Set("url", r.URL.Path))
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("getSignedJWTTokenCookie LogAudit: Login %v %v",
			claims.Username, r.RemoteAddr)
	}

	// Sets the cookie on the browser so it is only valid from the
	// base down.
	return &http.Cookie{
		Name:     "VelociraptorAuth",
		Value:    tokenString,
		Path:     api_utils.GetBaseDirectory(config_obj),
		Secure:   true,
		HttpOnly: true,
		Expires:  expiry,
	}, nil
}

// Extracts the claims from the VelociraptorAuth cookie:
// Ensure the JWT is properly validated and contains all the
// required fields.
func getDetailsFromCookie(
	config_obj *config_proto.Config,
	r *http.Request) (*Claims, error) {

	claims := &Claims{}

	// We store the user name and their details in a local
	// cookie. It is stored as a JWT so we can trust it.
	auth_cookie, err := r.Cookie("VelociraptorAuth")
	if err != nil {
		return claims, reauthError
	}

	// Parse the JWT.
	token, err := jwt.ParseWithClaims(auth_cookie.Value, claims,
		func(token *jwt.Token) (interface{}, error) {
			_, ok := token.Method.(*jwt.SigningMethodHMAC)
			if !ok {
				return claims, errors.New("invalid signing method")
			}
			return []byte(config_obj.Frontend.PrivateKey), nil
		})
	if err != nil {
		return claims, fmt.Errorf("%w: %v", err, reauthError.Error())
	}

	claims, ok := token.Claims.(*Claims)
	if ok && token.Valid {
		return claims, nil
	}

	return claims, reauthError
}
