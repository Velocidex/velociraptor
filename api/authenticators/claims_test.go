package authenticators

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

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

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader([]byte(resp))),
		}, nil
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
			// transparently fixed support. We test is anyway.
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
	}
)

type TestRouter struct {
	DefaultOidcRouter
}

func (self *TestRouter) Issuer() string {
	return "https://www.example.com"
}

func TestProvider(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	golden := ordereddict.NewDict()

	for _, tc := range testUrlReplayerCases {
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
		g.Set("Cookie", cookie)
		g.Set("Claims", claims)
	}

	goldie.Assert(t, "TestProvider", json.MustMarshalIndent(golden))
}
