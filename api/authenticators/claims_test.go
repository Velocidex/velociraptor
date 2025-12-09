package authenticators

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

// Replay responses to HTTP requests. Allows us to mock out the HTTP
// transactions.
type StaticURLReplayer struct {
	name      string
	err_regex string
	responses map[string]string
	URLs      []string

	authenticator *config_proto.Authenticator
}

func (self *StaticURLReplayer) GetRoundTrip(rt RoundTripFunc) RoundTripFunc {
	return func(req *http.Request) (*http.Response, error) {
		resp, pres := self.responses[req.URL.String()]
		if !pres {
			fmt.Printf("Unknown request: %v\n", req.URL.String())
		}
		resp = strings.TrimSpace(resp)

		result := &http.Response{
			StatusCode: 200,
			Header:     make(map[string][]string),
			Body:       io.NopCloser(bytes.NewReader([]byte(resp))),
		}

		if strings.HasPrefix(resp, "{") {
			result.Header["Content-Type"] = []string{
				"application/json; charset=utf-8"}
		} else {
			result.Header["Content-Type"] = []string{
				"application/x-www-form-urlencoded; charset=utf-8"}
		}

		return result, nil
	}
}

var (
	testUrlReplayerCases = []StaticURLReplayer{
		StaticURLReplayer{
			name: "Typical Flow",
			responses: map[string]string{
				// These are the responses for a typical oidc flow:

				// 1. First the provider queries the well known config endpoint.
				`https://www.example.com/.well-known/openid-configuration`: `
{"issuer": "https://www.example.com",
 "authorization_endpoint": "https://www.example.com/o/oauth2/v2/auth",
 "token_endpoint": "https://www.example.com/token",
 "userinfo_endpoint": "https://www.example.com/v1/userinfo"
}
`,
				// 2. Next we get the token
				"https://www.example.com/token": `
{"access_token": "This is an access token",
 "expires_in": 3598,
 "scope": "openid https://www.googleapis.com/auth/userinfo.email",
 "token_type": "Bearer",
 "id_token": "XXXX"
}
`,
				// 3. Finally we fetch the user info endpoint.
				"https://www.example.com/v1/userinfo": `
{"sub": "100439259231459671911",
 "email": "user@example.com",
 "email_verified": true
}
`,
			},
		},

		StaticURLReplayer{
			// AWS Cognito does not follow the spec and encode
			// email_verified as a string. We used to have special
			// handling for it but it seems now the upstream library
			// transparently fixed support. We test it anyway.
			name: "AWS congnito",
			responses: map[string]string{
				// These are the responses for a typical oidc flow:

				// 1. First the provider queries the well known config endpoint.
				`https://www.example.com/.well-known/openid-configuration`: `
{"issuer": "https://www.example.com",
 "authorization_endpoint": "https://www.example.com/o/oauth2/v2/auth",
 "token_endpoint": "https://www.example.com/token",
 "userinfo_endpoint": "https://www.example.com/v1/userinfo"
}
`,
				// 2. Next we get the token
				"https://www.example.com/token": `
{"access_token": "This is an access token",
 "expires_in": 3598,
 "scope": "openid https://www.googleapis.com/auth/userinfo.email",
 "token_type": "Bearer",
 "id_token": "XXXX"
}
`,
				// 3. Finally we fetch the user info endpoint.
				"https://www.example.com/v1/userinfo": `
{"sub": "100439259231459671911",
 "email": "user@example.com",
 "email_verified": "true"
}
`,
			},
		},

		StaticURLReplayer{
			// ADFS seems to encode the user info in the AccessToken
			// itself and returns a useless response to the UserInfo
			// endpoint. We support this behavior implicitly.
			name: "MS ADFS",
			responses: map[string]string{
				`https://www.example.com/.well-known/openid-configuration`: `
{"issuer": "https://www.example.com",
 "authorization_endpoint": "https://www.example.com/o/oauth2/v2/auth",
 "token_endpoint": "https://www.example.com/token",
 "userinfo_endpoint": "https://www.example.com/v1/userinfo"
}
`,
				// access_token contains a JWT with the email claim.
				"https://www.example.com/token": `
{"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhcHBpZCI6IngxMjMiLCJlbWFpbCI6Im5vb25lQG5vd2hlcmUuY29tIiwiZXhwIjoxNzY0OTM3NDgzLCJzY3AiOiJlbWFpbCBvcGVuaWQifQ.3AjZi1Cxpbvscxh6HrDQ9iHOwom6G9RP-iRhv3vHLBo",
 "expires_in": 3598,
 "scope": "openid https://www.googleapis.com/auth/userinfo.email",
 "token_type": "Bearer",
 "id_token": "XXXX"
}`,

				// ADFS sends a minimal userinfo because most of the
				// claims are encoded in the token.
				"https://www.example.com/v1/userinfo": `
{"sub": "100439259231459671911"}`,
			},
		},

		StaticURLReplayer{
			// Usually we require the email_verified claim to be able
			// to use the email but some IDPs do not set it. We reject
			// such uers.
			name:      "Email is not verified",
			err_regex: "Email .+ is not verified",
			responses: map[string]string{
				`https://www.example.com/.well-known/openid-configuration`: `
{"issuer": "https://www.example.com",
 "authorization_endpoint": "https://www.example.com/o/oauth2/v2/auth",
 "token_endpoint": "https://www.example.com/token",
 "userinfo_endpoint": "https://www.example.com/v1/userinfo"
}
`,
				"https://www.example.com/token": `
{"access_token": "This is an access token",
 "expires_in": 3598,
 "scope": "openid https://www.googleapis.com/auth/userinfo.email",
 "token_type": "Bearer",
 "id_token": "XXXX"
}
`,
				"https://www.example.com/v1/userinfo": `
{"sub": "100439259231459671911",
 "email": "user@example.com",
 "email_verified": false
}
`,
			},
		},
		StaticURLReplayer{
			// Usually we require the email_verified claim to be able
			// to use the email but some IDPs do not set it. We reject
			// such uers.
			name: "Email is not verified but it is allowed",
			authenticator: &config_proto.Authenticator{
				Type:              "oidc",
				OauthClientId:     "ClientIdXXXX",
				OauthClientSecret: "ClientSecrect1234",
				OidcIssuer:        "https://www.example.com",
				Claims: &config_proto.OIDCClaims{
					AllowUnverifiedEmail: true,
				},
			},
			responses: map[string]string{
				`https://www.example.com/.well-known/openid-configuration`: `
{"issuer": "https://www.example.com",
 "authorization_endpoint": "https://www.example.com/o/oauth2/v2/auth",
 "token_endpoint": "https://www.example.com/token",
 "userinfo_endpoint": "https://www.example.com/v1/userinfo"
}
`,
				"https://www.example.com/token": `
{"access_token": "This is an access token",
 "expires_in": 3598,
 "scope": "openid https://www.googleapis.com/auth/userinfo.email",
 "token_type": "Bearer",
 "id_token": "XXXX"
}
`,
				"https://www.example.com/v1/userinfo": `
{"sub": "100439259231459671911",
 "email": "user@example.com",
 "email_verified": false
}
`,
			},
		},
		StaticURLReplayer{
			// Usually we use the email claim but some IDPs use other
			// claims to identify the user.
			name: "Unusual claim name",
			authenticator: &config_proto.Authenticator{
				Type:              "oidc",
				OauthClientId:     "ClientIdXXXX",
				OauthClientSecret: "ClientSecrect1234",
				OidcIssuer:        "https://www.example.com",
				Claims: &config_proto.OIDCClaims{
					Username: "SomeWeirdClaim",
				},
			},
			responses: map[string]string{
				`https://www.example.com/.well-known/openid-configuration`: `
{"issuer": "https://www.example.com",
 "authorization_endpoint": "https://www.example.com/o/oauth2/v2/auth",
 "token_endpoint": "https://www.example.com/token",
 "userinfo_endpoint": "https://www.example.com/v1/userinfo"
}
`,
				"https://www.example.com/token": `
{"access_token": "This is an access token",
 "expires_in": 3598,
 "scope": "openid https://www.googleapis.com/auth/userinfo.email",
 "token_type": "Bearer"
}
`,
				"https://www.example.com/v1/userinfo": `
{"sub": "100439259231459671911",
 "SomeWeirdClaim": "user@example.com"
}
`,
			},
		},
		StaticURLReplayer{
			name: "Google Authenticator",
			authenticator: &config_proto.Authenticator{
				Type:              "google",
				OauthClientId:     "ClientIdXXXX",
				OauthClientSecret: "ClientSecrect1234",
			},
			responses: map[string]string{
				`https://accounts.google.com/.well-known/openid-configuration`: `
{
 "issuer": "https://accounts.google.com",
 "authorization_endpoint": "https://accounts.google.com/o/oauth2/v2/auth",
 "device_authorization_endpoint": "https://oauth2.googleapis.com/device/code",
 "token_endpoint": "https://oauth2.googleapis.com/token",
 "userinfo_endpoint": "https://openidconnect.googleapis.com/v1/userinfo",
 "revocation_endpoint": "https://oauth2.googleapis.com/revoke",
 "jwks_uri": "https://www.googleapis.com/oauth2/v3/certs"
}
`,
				"https://oauth2.googleapis.com/token": `
{"access_token": "This is an access token",
 "expires_in": 3598,
 "scope": "openid https://www.googleapis.com/auth/userinfo.email",
 "token_type": "Bearer"
}
`,
				"https://openidconnect.googleapis.com/v1/userinfo": `
{"sub": "100439259231459671911",
 "picture": "https://lh3.googleusercontent.com/a-/XXXXXX",
  "email": "user@example.com",
  "email_verified": true,
  "hd": "example.com"
}
`,
			},
		},
		StaticURLReplayer{
			name: "Github Authenticator",
			authenticator: &config_proto.Authenticator{
				Type:              "github",
				OauthClientId:     "ClientIdXXXX",
				OauthClientSecret: "ClientSecrect1234",
			},
			responses: map[string]string{
				"https://github.com/login/oauth/access_token": `access_token=XXXX&scope=user%3Aemail&token_type=bearer`,
				"https://api.github.com/user": `
{"login":"gh_user",
  "id":12345,
  "node_id":"XXXX=",
  "avatar_url":"https://avatars.githubusercontent.com/u/3856546?v=4",
  "gravatar_id":"","url":"https://api.github.com/users/gh_user",
  "html_url":"https://github.com/gh_user",
  "followers_url":"https://api.github.com/users/gh_user/followers",
  "following_url":"https://api.github.com/users/gh_user/following{/other_user}",
  "gists_url":"https://api.github.com/users/gh_user/gists{/gist_id}",
  "starred_url":"https://api.github.com/users/gh_user/starred{/owner}{/repo}",
  "subscriptions_url":"https://api.github.com/users/gh_user/subscriptions",
  "organizations_url":"https://api.github.com/users/gh_user/orgs",
  "repos_url":"https://api.github.com/users/gh_user/repos",
  "events_url":"https://api.github.com/users/gh_user/events{/privacy}",
  "received_events_url":"https://api.github.com/users/gh_user/received_events",
  "type":"User",
  "user_view_type":"public",
  "site_admin":false,
  "name":"Mike Cohen",
  "company":"@Velocidex ",
  "blog":"",
  "location":"Australia"
}
`,
			},
		},
		StaticURLReplayer{
			name: "Azure Authenticator",
			authenticator: &config_proto.Authenticator{
				Type:              "azure",
				OauthClientId:     "ClientIdXXXX",
				OauthClientSecret: "ClientSecrect1234",
				Tenant:            "F1234",
			},
			responses: map[string]string{
				"https://login.microsoftonline.com/F1234/oauth2/v2.0/token": `
{"token_type":"Bearer",
 "scope":"profile User.Read openid email",
 "expires_in":4471,
 "ext_expires_in":4471,
 "access_token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhcHBpZCI6IngxMjMiLCJlbWFpbCI6Im5vb25lQG5vd2hlcmUuY29tIiwiZXhwIjoxNzY0OTM3NDgzLCJzY3AiOiJlbWFpbCBvcGVuaWQifQ.3AjZi1Cxpbvscxh6HrDQ9iHOwom6G9RP-iRhv3vHLBo"
}`,
				"https://graph.microsoft.com/v1.0/me/": `
{"@odata.context":"https://graph.microsoft.com/v1.0/$metadata#users/$entity",
 "businessPhones":["0470238491"],
 "displayName":"Mike Cohen",
 "givenName":"Mike",
 "jobTitle":null,
 "mail":"user@example.com",
 "mobilePhone":null,
 "officeLocation":null,
 "preferredLanguage":"en-US",
 "surname":"Cohen",
 "userPrincipalName":"user@example.com",
 "id":"bcb8e2a7-b2d6-49e1-a6c0-327fc8ea1058"}
`,
				"https://graph.microsoft.com/v1.0/me/photos/48x48/$value": `My Picture`,
			},
		},
	}
)

type TestRouter struct {
	DefaultOidcRouter
}

func (self *TestRouter) Issuer() string {
	return "https://www.example.com"
}

type OauthTestSuire struct {
	test_utils.TestSuite
}

func (self *OauthTestSuire) TestProvider() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(1765349444, 0)))
	defer closer()

	t := self.T()

	config_obj := config.GetDefaultConfig()
	golden := ordereddict.NewDict()

	for _, tc := range testUrlReplayerCases {
		if false && tc.name != "Azure Authenticator" {
			continue
		}

		g := ordereddict.NewDict()
		golden.Set(tc.name, g)

		authenticator := tc.authenticator
		if authenticator == nil {
			authenticator = &config_proto.Authenticator{
				Type:              "oidc",
				OauthClientId:     "ClientIdXXXX",
				OauthClientSecret: "ClientSecrect1234",
				OidcIssuer:        "https://www.example.com",
			}
		}

		ctx, err := ClientContext(
			context.Background(), config_obj,
			[]Transformer{tc.GetRoundTrip})
		assert.NoError(t, err)

		auther, err := getAuthenticatorByType(ctx, config_obj, authenticator)
		assert.NoError(t, err)

		oidc_auther, ok := auther.(*OidcAuthenticator)
		assert.True(t, ok)

		provider, err := oidc_auther.Provider()
		assert.NoError(t, err)

		// See the redirect URL.
		redirect := provider.GetRedirectURL(nil, "StateString")
		g.Set("Redirect URL", redirect)

		cookie, claims, err := provider.GetJWT(ctx, "code")
		if tc.err_regex != "" {
			assert.Regexp(t, tc.err_regex, err.Error())
			continue
		}
		assert.NoError(t, err)
		cookie.Value = fmt.Sprintf("String of length %v", len(cookie.Value))

		g.Set("Cookie", cookie)
		g.Set("Claims", claims)
	}

	goldie.Assert(t, "TestProvider", json.MustMarshalIndent(golden))
}

func TestOIDC(t *testing.T) {
	suite.Run(t, &OauthTestSuire{})
}
