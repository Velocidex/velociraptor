package ssh

import (
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"
)

type SSHSessionTestSuite struct {
	test_utils.TestSuite
}

// makeScope returns a scope that looks enough like a server side scope
// for getSecret() to work - it has a server config in the cache and a
// principal that the secret below is granted to.
func (self *SSHSessionTestSuite) makeScope() vfilter.Scope {
	scope := vql_subsystem.MakeScope()
	scope.AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	vql_subsystem.CacheSet(scope, constants.SCOPE_SERVER_CONFIG, self.ConfigObj)
	return scope
}

func (self *SSHSessionTestSuite) addSecret(
	scope vfilter.Scope, name string, data *ordereddict.Dict) {
	secrets_service, err := services.GetSecretsService(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = secrets_service.AddSecret(self.Ctx, scope,
		constants.SSH_PRIVATE_KEY, name, data)
	assert.NoError(self.T(), err)

	err = secrets_service.ModifySecret(self.Ctx,
		&api_proto.ModifySecretRequest{
			TypeName: constants.SSH_PRIVATE_KEY,
			Name:     name,
			AddUsers: []string{constants.PinnedServerName}})
	assert.NoError(self.T(), err)
}

// The reported bug: an explicit hostkey in SSH_CONFIG was silently
// dropped whenever secret= was also specified, because getSecret()
// returned a brand new args struct which only carried the credentials.
func (self *SSHSessionTestSuite) TestHostKeySurvivesSecret() {
	scope := self.makeScope()

	// A secret with no host key of its own - this is the common case
	// for secrets created before the hostkey field existed.
	self.addSecret(scope, "NoHostKey", ordereddict.NewDict().
		Set("username", "fred").
		Set("hostname", "myhost.com").
		Set("password", "hunter2"))

	secret_args, err := getSecret(self.Ctx, scope, "NoHostKey")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "", secret_args.HostKey)

	// The caller pinned a host key in SSH_CONFIG - it must survive the
	// merge, otherwise the pinning is silently a no-op.
	config_args := &SSHAccessorArgs{
		Secret:  "NoHostKey",
		HostKey: "CONFIG_HOST_KEY",
	}

	merged := mergeSecretArgs(config_args, secret_args)
	assert.Equal(self.T(), "CONFIG_HOST_KEY", merged.HostKey)

	// The secret still wins for the connection details.
	assert.Equal(self.T(), "myhost.com", merged.Hostname)
	assert.Equal(self.T(), "fred", merged.Username)
	assert.Equal(self.T(), "hunter2", merged.Password)
}

// A secret may also carry the host key itself. The secret is the
// authoritative record of how to reach the remote system, so its host
// key is not overridable from SSH_CONFIG.
func (self *SSHSessionTestSuite) TestHostKeyFromSecretWins() {
	scope := self.makeScope()

	self.addSecret(scope, "WithHostKey", ordereddict.NewDict().
		Set("username", "fred").
		Set("hostname", "myhost.com").
		Set("password", "hunter2").
		Set("hostkey", "SECRET_HOST_KEY"))

	secret_args, err := getSecret(self.Ctx, scope, "WithHostKey")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "SECRET_HOST_KEY", secret_args.HostKey)

	merged := mergeSecretArgs(
		&SSHAccessorArgs{HostKey: "CONFIG_HOST_KEY"}, secret_args)
	assert.Equal(self.T(), "SECRET_HOST_KEY", merged.HostKey)
}

func TestSSHSession(t *testing.T) {
	suite.Run(t, &SSHSessionTestSuite{})
}
