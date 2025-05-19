package authenticators

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Velocidex/ordereddict"
	jwt "github.com/golang-jwt/jwt/v4"
	utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	reauthError = errors.New(`Authentication cookie not found, invalid or expired.
You probably need to re-authenticate in a new tab or refresh this page.`)
)

func getSignedJWTTokenCookie(
	config_obj *config_proto.Config,
	authenticator *config_proto.Authenticator,
	claims *Claims,
	r *http.Request) (*http.Cookie, error) {
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
	expiry := time.Now().Add(time.Minute * time.Duration(expiry_min))

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
	services.LogAudit(r.Context(),
		config_obj, claims.Username, "Login",
		ordereddict.NewDict().
			Set("remote", r.RemoteAddr).
			Set("authenticator", authenticator.Type).
			Set("url", r.URL.Path))

	// Sets the cookie on the browser so it is only valid from the
	// base down.
	return &http.Cookie{
		Name:     "VelociraptorAuth",
		Value:    tokenString,
		Path:     utils.GetBaseDirectory(config_obj),
		Secure:   true,
		HttpOnly: true,
		Expires:  expiry,
	}, nil
}

// Ensure the JWT is properly validated and contains all the required fields.
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
