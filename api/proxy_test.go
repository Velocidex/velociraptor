package api

import (
	"fmt"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type APIProxyTestSuite struct {
	test_utils.TestSuite
}

func (self *APIProxyTestSuite) TestMultiAuthenticator() {
	authenticators.ResetAuthCache()

	mux := api_utils.NewServeMux()

	config_obj := proto.Clone(self.ConfigObj).(*config_proto.Config)
	config_obj.GUI.PublicUrl = "https://www.example.com/"
	config_obj.GUI.BasePath = "/velociraptor"
	config_obj.GUI.Authenticator = &config_proto.Authenticator{
		Type: "multi",
		SubAuthenticators: []*config_proto.Authenticator{{
			Type:              "oidc",
			OidcIssuer:        "https://accounts.google.com",
			OauthClientId:     "CCCCC",
			OauthClientSecret: "secret",
		}, {
			Type:              "Google",
			OauthClientId:     "CCCCC",
			OauthClientSecret: "secret",
		}, {
			Type:              "GitHub",
			OauthClientId:     "CCCCC",
			OauthClientSecret: "secret",
		}, {
			Type:              "azure",
			OauthClientId:     "CCCCC",
			OauthClientSecret: "secret",
		}},
	}

	_, err := PrepareGUIMux(self.Ctx, config_obj, mux)
	assert.NoError(self.T(), err)

	auther, err := authenticators.NewAuthenticator(config_obj)
	assert.NoError(self.T(), err)

	auther_multi, ok := auther.(*authenticators.MultiAuthenticator)
	assert.True(self.T(), ok)

	golden := ordereddict.NewDict()

	for _, delegate := range auther_multi.Delegates() {
		auther_oidc, ok := delegate.(*authenticators.OidcAuthenticator)
		if !ok {
			continue
		}

		oidc_config, err := auther_oidc.GetGenOauthConfig()
		assert.NoError(self.T(), err)
		golden.Set(fmt.Sprintf("Redirect Provider %T %v", delegate, auther_oidc.Name()),
			oidc_config.RedirectURL)
	}

	golden.Set("Mux", mux.Debug())

	goldie.Assert(self.T(), "TestMultiAuthenticator", json.MustMarshalIndent(golden))
}

func (self *APIProxyTestSuite) TestBasicAuthenticator() {
	authenticators.ResetAuthCache()

	mux := api_utils.NewServeMux()

	config_obj := proto.Clone(self.ConfigObj).(*config_proto.Config)
	config_obj.GUI.PublicUrl = "https://www.example.com/"
	config_obj.GUI.BasePath = "/velociraptor"
	config_obj.GUI.Authenticator = &config_proto.Authenticator{
		Type: "basic",
	}

	_, err := PrepareGUIMux(self.Ctx, config_obj, mux)
	assert.NoError(self.T(), err)

	golden := ordereddict.NewDict()

	golden.Set("Mux", mux.Debug())

	goldie.Assert(self.T(), "TestBasicAuthenticator", json.MustMarshalIndent(golden))
}

func TestAPIProxy(t *testing.T) {
	suite.Run(t, &APIProxyTestSuite{})
}
