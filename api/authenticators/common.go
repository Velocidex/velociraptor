package authenticators

import (
	"errors"
	"net/http"
	"time"

	jwt "github.com/golang-jwt/jwt"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type Claims struct {
	Username string  `json:"username"`
	Picture  string  `json:"picture"`
	Expires  float64 `json:"expires"`
	Token    string  `json:"token"`
}

func (self *Claims) Valid() error {
	if self.Username == "" {
		return errors.New("username not present")
	}

	if self.Expires < float64(time.Now().Unix()) {
		return errors.New("the JWT is expired - reauthenticate")
	}
	return nil
}

func getSignedJWTTokenCookie(
	config_obj *config_proto.Config,
	authenticator *config_proto.Authenticator,
	claims *Claims) (*http.Cookie, error) {
	if config_obj.Frontend == nil {
		return nil, errors.New("config has no Frontend")
	}

	expiry_min := authenticator.DefaultSessionExpiryMin
	if expiry_min == 0 {
		expiry_min = 60 * 24 // 1 Day by default
	}

	// We force expiry in the JWT **as well** as the session
	// cookie. The JWT expiry is most important as the browser can
	// replay sessioon cookies past expiry.
	expiry := time.Now().Add(time.Minute * time.Duration(expiry_min))

	// Enfore the JWT to expire
	claims.Expires = float64(expiry.Unix())

	// Make a JWT and sign it.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(config_obj.Frontend.PrivateKey))
	if err != nil {
		return nil, err
	}

	// Sets the cookie on the browser so it is only valid from the
	// base down.
	base := getBasePath(config_obj)

	return &http.Cookie{
		Name:     "VelociraptorAuth",
		Value:    tokenString,
		Path:     base,
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
		return claims, err
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
		return claims, err
	}

	claims, ok := token.Claims.(*Claims)
	if ok && token.Valid {
		return claims, nil
	}

	return claims, err
}
