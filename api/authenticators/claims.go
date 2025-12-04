package authenticators

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/Velocidex/ordereddict"
	oidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
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

	if self.Expires < float64(time.Now().Unix()) {
		return errors.New("the JWT is expired - reauthenticate")
	}
	return nil
}

func (self *OidcAuthenticator) maybeGetClaimsFromToken(
	ctx context.Context, token *oauth2.Token) (*Claims, error) {
	data, err := base64.StdEncoding.DecodeString(token.AccessToken)
	if err != nil {
		return nil, utils.InvalidArgError
	}

	if self.authenticator.OidcDebug {
		logging.GetLogger(self.config_obj, &logging.GUIComponent).
			Debug("OidcAuthenticator: Getting claims from access token: %s", data)
	}

	claims := ordereddict.NewDict()
	err = json.Unmarshal(data, claims)
	if err != nil {
		return nil, utils.InvalidArgError
	}

	return self.newClaimsFromDict(ctx, claims)
}

func (self *OidcAuthenticator) NewClaims(
	ctx context.Context, user_info *oidc.UserInfo) (*Claims, error) {
	claims := ordereddict.NewDict()
	err := user_info.Claims(&claims)
	if err != nil {
		return nil, err
	}

	return self.newClaimsFromDict(ctx, claims)
}

func (self *OidcAuthenticator) newClaimsFromDict(
	ctx context.Context, claims *ordereddict.Dict) (*Claims, error) {

	if self.authenticator.OidcDebug {
		logging.GetLogger(self.config_obj, &logging.GUIComponent).
			Debug("OidcAuthenticator: Parsing claims from OIDC Claims: %#v", claims)
	}

	username_field := "email"
	roles_field := ""

	if self.authenticator.Claims != nil {
		if self.authenticator.Claims.Username != "" {
			username_field = self.authenticator.Claims.Username
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

	logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)

	// First check the user exist at all.
	_, err := user_manager.GetUser(ctx, email, email)
	if utils.IsNotFound(err) {
		// If the user does not exist at all, create it.
		user_record := &api_proto.VelociraptorUser{
			Name: email,
		}

		err = services.LogAudit(ctx, self.config_obj, email,
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
				if self.authenticator.OidcDebug {
					logging.GetLogger(self.config_obj, &logging.GUIComponent).
						Debug("No allowed claim role map for OIDC claim %#v", oidc_role)
				}
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
				err = services.LogAudit(ctx, self.config_obj, email,
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
