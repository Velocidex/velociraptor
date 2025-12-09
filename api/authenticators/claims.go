package authenticators

import (
	"context"
	"errors"
	"fmt"

	"github.com/Velocidex/ordereddict"
	oidc "github.com/coreos/go-oidc/v3/oidc"
	jwt "github.com/golang-jwt/jwt/v4"
	"golang.org/x/oauth2"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// The claims we care about - these are mapped from the IDP's claims
// using the oidc configuration.
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

	if self.Expires < float64(utils.GetTime().Now().Unix()) {
		return errors.New("the JWT is expired - reauthenticate")
	}
	return nil
}

// A ClaimsGetter is responsible for fetching a claim from the oauth
// server. Depending on the server type the claims are encoded
// differently or fetched from different locations using different
// methods.
type ClaimsGetter interface {
	GetClaims(ctx *HTTPClientContext, token *oauth2.Token) (*Claims, error)
}

// The ClaimsGetter for standard OIDC endpoints. This fetches the
// claims from:
//  1. The standard OIDC UserInfo endpoint
//  2. Attempts to decode the claim from the AccessToken if it is a JWT.
//     This behaviour was observed on ADFS.
type OidcClaimsGetter struct {
	config_obj    *config_proto.Config
	authenticator *config_proto.Authenticator
	router        OidcRouter

	provider *oidc.Provider
}

func NewOidcClaimsGetter(
	ctx *HTTPClientContext,
	config_obj *config_proto.Config,
	authenticator *config_proto.Authenticator,
	router OidcRouter) (*OidcClaimsGetter, error) {

	delegate, err := oidc.NewProvider(ctx, router.Issuer())
	if err != nil {
		return nil, err
	}
	ep := delegate.Endpoint()
	router.SetEndpoint(ep)

	return &OidcClaimsGetter{
		config_obj:    config_obj,
		authenticator: authenticator,
		router:        router,
		provider:      delegate,
	}, nil
}

func (self *OidcClaimsGetter) maybeGetClaimsFromToken(
	ctx context.Context, token *oauth2.Token) (*ordereddict.Dict, error) {

	// The token came from the ADFS server and will be used again to
	// get the UserInfo so it must be valid. We do not need to check
	// its signature.
	claims := jwt.MapClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(token.AccessToken, claims)
	if err != nil {
		return nil, err
	}
	res := ordereddict.NewDict()
	for k, v := range claims {
		res.Set(k, v)
	}
	return res, nil
}

func (self *OidcClaimsGetter) UserInfo(
	ctx context.Context,
	token *oauth2.Token) (*oidc.UserInfo, error) {
	user_info, err := self.provider.UserInfo(
		ctx, oauth2.StaticTokenSource(token))
	if err != nil {
		return nil, err
	}
	return user_info, err
}

func (self *OidcClaimsGetter) Debug(message string, args ...interface{}) {
	if self.authenticator.OidcDebug {
		logging.GetLogger(self.config_obj, &logging.GUIComponent).
			Debug(message, args...)
	}
}

func (self *OidcClaimsGetter) GetClaims(
	ctx *HTTPClientContext, token *oauth2.Token) (claims *Claims, err error) {

	claims_dict, err := self.getClaims(ctx, token)
	if err != nil {
		self.Debug("Unable to parse claims from user info: %v", err)

		// Fallsback to try to get the claims from the token
		token_claims_dict, err1 := self.maybeGetClaimsFromToken(ctx, token)
		if err1 != nil {
			return nil, err
		}
		self.Debug("Unwrapped claims from AccessToken: %v", token_claims_dict)
		claims_dict = token_claims_dict
	}

	return self.newClaimsFromDict(ctx, self.config_obj, claims_dict)
}

func (self *OidcClaimsGetter) shouldRequireEmailVerify(
	authenticator *config_proto.Authenticator) bool {
	if authenticator.Claims == nil {
		return true
	}

	if authenticator.Claims.AllowUnverifiedEmail {
		return false
	}

	// If the user wants to use a different claim than email then
	// email verified is not relevant.
	if authenticator.Claims.Username != "" {
		return false
	}

	return true
}

func (self *OidcClaimsGetter) getClaims(
	ctx context.Context, token *oauth2.Token) (claims *ordereddict.Dict, err error) {

	user_info, err := self.UserInfo(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("can not get UserInfo from OIDC provider: %v", err)
	}

	// Make sure the user's email is verified because this is what we
	// use as the identity.
	if !user_info.EmailVerified &&
		self.shouldRequireEmailVerify(self.authenticator) {
		return nil, fmt.Errorf("Email %v is not verified", user_info.Email)
	}

	claims = ordereddict.NewDict()
	err = user_info.Claims(&claims)
	if err != nil {
		return nil, err
	}

	return claims, nil
}

func (self *OidcClaimsGetter) newClaimsFromDict(
	ctx context.Context,
	config_obj *config_proto.Config,
	claims *ordereddict.Dict) (*Claims, error) {

	username_field := "email"
	roles_field := ""

	if self.authenticator.Claims != nil {
		if self.authenticator.Claims.Username != "" {
			username_field = self.authenticator.Claims.Username
			self.Debug("Using field %v in claims for username", username_field)
		}

		if self.authenticator.Claims.Roles != "" {
			roles_field = self.authenticator.Claims.Roles
		}
	}

	email, _ := claims.GetString(username_field)
	if email == "" {
		return nil, fmt.Errorf(
			"OidcAuthenticator: Unable to parse name claim using field %v: %v",
			username_field, claims)
	}

	res := &Claims{
		Username: email,
	}

	if self.authenticator.Claims == nil ||
		self.authenticator.Claims.RoleMap == nil ||
		roles_field == "" {
		return res, nil
	}

	roles, pres := claims.GetStrings(roles_field)
	if !pres {
		return res, nil

	}

	user_manager := services.GetUserManager()

	logger := logging.GetLogger(config_obj, &logging.GUIComponent)

	// First check the user exist at all.
	_, err := user_manager.GetUser(ctx, email, email)
	if utils.IsNotFound(err) {
		// If the user does not exist at all, create it.
		user_record := &api_proto.VelociraptorUser{
			Name: email,
		}

		err = services.LogAudit(ctx, config_obj, email,
			"Create User From OIDC Roles",
			ordereddict.NewDict().Set("Claims", claims))
		if err != nil {
			return nil, err
		}

		err = user_manager.SetUser(ctx, user_record)
		if err != nil {
			return nil, err
		}

		// Some other error occured - reject.
	} else if err != nil {
		return nil, err
	}

	// Usually roles are set per org but setting roles through the
	// OIDC IDP will grant the roles on all orgs.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	for _, org := range org_manager.ListOrgs() {
		org_config_obj, err := org_manager.GetOrgConfig(org.Id)
		if err != nil {
			continue
		}

		// Get the user's ACL policy in that org
		existing_acls, err := services.GetPolicy(org_config_obj, email)
		if err != nil {
			// If a user does not exist this will fail to get their
			// policy so start with a fresh policy.
			existing_acls = &acl_proto.ApiClientACL{}
		}

		for _, oidc_role := range roles {
			acl_spec, pres := self.authenticator.Claims.RoleMap[oidc_role]
			if !pres {
				self.Debug("No allowed claim role map for OIDC claim %#v", oidc_role)
				continue
			}

			var new_roles []string
			for _, role := range acl_spec.Roles {
				if !utils.InString(existing_acls.Roles, role) {
					new_roles = append(new_roles, role)
				}
			}

			// Merge old roles
			for _, role := range existing_acls.Roles {
				if !utils.InString(new_roles, role) {
					new_roles = append(new_roles, role)
				}
			}

			// Only set the roles if we need to - note we can only
			// ever add roles to the existing roles.
			if len(new_roles) > len(existing_acls.Roles) {
				err = services.LogAudit(ctx, config_obj, email,
					"Grant User Role From OIDC Claim",
					ordereddict.NewDict().
						Set("Roles", new_roles).
						Set("OrgId", org.Id).
						Set("Claims", claims))
				if err != nil {
					continue
				}

				logger.Info("Granting roles %v to User %v in org %v",
					new_roles, email, org.Id)
				err = services.GrantRoles(org_config_obj, email, new_roles)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return res, nil
}
