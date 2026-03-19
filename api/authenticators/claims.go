package authenticators

import (
	"context"
	"errors"
	"fmt"

	"github.com/Velocidex/ordereddict"
	oidc "github.com/coreos/go-oidc/v3/oidc"
	jwt "github.com/golang-jwt/jwt/v4"
	"golang.org/x/oauth2"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
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

	ignore_id_token bool
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

	res := ordereddict.NewDict()

	// The AccessToken is supposed to be opaque since it is used as a
	// bearer token by the API. We do not need to actually validate it.

	// On ADFS, this token can be decoded and actually contains some
	// information about the user so we try to get that info anyway.
	claims := jwt.MapClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(token.AccessToken, claims)
	if err == nil {
		for k, v := range claims {
			res.Set(k, v)
		}
	}

	// Usually only used in tests - real IDPs should provide an ID
	// token
	if self.ignore_id_token {
		return res, nil
	}

	// The real claim is sent in the ID token
	oidcConfig := &oidc.Config{
		ClientID: self.authenticator.OauthClientId,
	}

	// https://github.com/Coreos/go-oidc/blob/v2.5.0/example/idtoken/app.go
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("No ID Token")
	}

	verifier := self.provider.Verifier(oidcConfig)
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, err
	}

	raw_id_token := make(map[string]interface{})
	err = idToken.Claims(&raw_id_token)
	if err != nil {
		return nil, err
	}

	// Merge the ID token into the AccessToken
	for k, v := range raw_id_token {
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

	claims_dict, err := self.maybeGetClaimsFromToken(ctx, token)
	if err == nil {
		// Try to parse the claims from the token
		res, err := self.newClaimsFromDict(ctx, self.config_obj, claims_dict)
		if err == nil {
			self.Debug("Unwrapped claims from AccessToken: %v", claims_dict)
			return res, nil
		}

	} else {
		self.Debug("Unable to parse claims from tokens: %v", err)
	}

	// If we cant get a valid claim from the token, we fallback to
	// try using the user info method
	claims_dict, err = self.getClaimsFromUserInfo(ctx, token)
	if err == nil {
		res, err := self.newClaimsFromDict(ctx, self.config_obj, claims_dict)
		if err == nil {
			self.Debug("Unwrapped claims from UserInfo: %v", claims_dict)
			return res, nil
		}

	} else {
		self.Debug("Unable to parse claims from user info: %v", err)
	}

	return nil, err
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

func (self *OidcClaimsGetter) getClaimsFromUserInfo(
	ctx context.Context, token *oauth2.Token) (claims *ordereddict.Dict, err error) {

	user_info, err := self.UserInfo(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("can not get UserInfo from OIDC provider: %v", err)
	}

	// Make sure the user's email is verified because this is what we
	// use as the identity.
	if self.shouldRequireEmailVerify(self.authenticator) &&
		!user_info.EmailVerified {
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
	if self.authenticator.Claims != nil &&
		self.authenticator.Claims.Username != "" {

		// Custom username field
		username_field = self.authenticator.Claims.Username
		self.Debug("Using field %v in claims for username", username_field)
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

	return res, self.SetRolesForUser(
		ctx, config_obj, email, claims)
}

func (self *OidcClaimsGetter) shouldUpdateACLs(
	new_acl, existing_acls *acl_proto.ApiClientACL,
) (*acl_proto.ApiClientACL, bool) {

	// When OverrideAcls is specified we just replace the
	// existing_acls with the new_acl if they are different.
	if self.authenticator.Claims.OverrideAcls {
		return new_acl, !acls.ACLEqual(new_acl, existing_acls)
	}

	// Merge the old ACL with the new ACL
	new_acl = acls.MergeACL(existing_acls, new_acl)
	return new_acl, !acls.ACLEqual(new_acl, existing_acls)
}

func (self *OidcClaimsGetter) SetRolesForUser(
	ctx context.Context,
	config_obj *config_proto.Config,
	email string,
	claims *ordereddict.Dict) error {

	// The roles field must be set to enable this feature!
	var roles_field string
	if self.authenticator.Claims != nil &&
		self.authenticator.Claims.Roles != "" {
		roles_field = self.authenticator.Claims.Roles

	} else {
		// Do nothing if automatic roles are not configured.
		return nil
	}

	// Do nothing if automatic roles are not configured.
	roles, pres := claims.GetStrings(roles_field)
	if !pres {
		return nil
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
			return err
		}

		err = user_manager.SetUser(ctx, user_record)
		if err != nil {
			return err
		}

		// Some other error occured - reject.
	} else if err != nil {
		return err
	}

	// Usually roles are set per org but setting roles through the
	// OIDC IDP will grant the roles on all orgs.
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
		existing_acls, err := services.GetPolicy(org_config_obj, email)
		if err != nil {
			// If a user does not exist this will fail to get their
			// policy so start with a fresh policy.
			existing_acls = &acl_proto.ApiClientACL{}
		}

		new_acl := &acl_proto.ApiClientACL{}

		// For each role give by the IDP we assign velociraptor roles
		for _, oidc_role := range roles {
			acl_spec, pres := self.authenticator.Claims.RoleMap[oidc_role]
			if !pres {
				self.Debug("No allowed claim role map for OIDC claim %#v", oidc_role)
				continue
			}

			for _, role := range acl_spec.Roles {
				if !utils.InString(new_acl.Roles, role) {
					new_acl.Roles = append(new_acl.Roles, role)
				}
			}
		}

		new_acl, should_update := self.shouldUpdateACLs(new_acl, existing_acls)
		if should_update {
			err = services.LogAudit(ctx, config_obj, email,
				"Grant User Role From OIDC Claim",
				ordereddict.NewDict().
					Set("ACL", new_acl).
					Set("OrgId", org.Id).
					Set("Claims", claims))
			if err != nil {
				continue
			}

			logger.Info("Granting acl %v to User %v in org %v",
				json.MustMarshalString(new_acl), email, org.Id)
			err = services.SetPolicy(org_config_obj, email, new_acl)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
