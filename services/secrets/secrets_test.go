package secrets_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"

	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/secrets"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type SecretsTestSuite struct {
	test_utils.TestSuite
}

func (self *SecretsTestSuite) TestSecretsService() {
	secrets, err := services.GetSecretsService(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Add an invalid secret - ssh requires more than just the
	// username to be set.
	scope := vql_subsystem.MakeScope()
	err = secrets.AddSecret(self.Ctx, scope,
		constants.SSH_PRIVATE_KEY, "MySecret", ordereddict.NewDict().
			Set("username", "fred"))
	assert.Error(self.T(), err)

	// For a valid secret we need more fields.
	err = secrets.AddSecret(self.Ctx, scope,
		constants.SSH_PRIVATE_KEY, "MySecret", ordereddict.NewDict().
			Set("username", "fred").
			Set("hostname", "myhost.com").
			Set("password", "hunter2").
			Set("extra_field", "This field will be removed"))
	assert.NoError(self.T(), err)

	golden := ordereddict.NewDict()
	db := test_utils.GetMemoryDataStore(self.T(), self.ConfigObj)

	golden.Set("Added Secret",
		getSecretFromStore(self.T(), self.ConfigObj, db,
			"config/secrets/"+constants.SSH_PRIVATE_KEY+"/MySecret"))

	// Grant the secret to two users
	err = secrets.ModifySecret(self.Ctx,
		&api_proto.ModifySecretRequest{
			TypeName: constants.SSH_PRIVATE_KEY,
			Name:     "MySecret",
			AddUsers: []string{"User1", "User2"}})
	assert.NoError(self.T(), err)

	golden.Set("Granted Secret",
		getSecretFromStore(self.T(), self.ConfigObj, db,
			"config/secrets/"+constants.SSH_PRIVATE_KEY+"/MySecret"))

	// User2 asks for the secret
	secret_data, err := secrets.GetSecret(
		self.Ctx, "User2", constants.SSH_PRIVATE_KEY, "MySecret")
	assert.NoError(self.T(), err)

	golden.Set("SecretData", secret_data)

	// Revoke user2
	err = secrets.ModifySecret(self.Ctx,
		&api_proto.ModifySecretRequest{
			TypeName:    constants.SSH_PRIVATE_KEY,
			Name:        "MySecret",
			RemoveUsers: []string{"User2"}})
	assert.NoError(self.T(), err)

	golden.Set("Revoked Secret",
		getSecretFromStore(self.T(), self.ConfigObj, db,
			"config/secrets/"+constants.SSH_PRIVATE_KEY+"/MySecret"))

	// User2 asks for the secret again - this time denied
	secret_data, err = secrets.GetSecret(
		self.Ctx, "User2", constants.SSH_PRIVATE_KEY, "MySecret")
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), `Permission Denied`)

	goldie.Assert(self.T(), "TestSecretsService",
		json.MustMarshalIndent(golden))
}

func verifyData(
	t *testing.T,
	config_obj *config_proto.Config,
	db *datastore.MemcacheDatastore,
	path_spec api.DSPathSpec) {

	b, _ := db.GetBuffer(config_obj, path_spec)

	result := ordereddict.NewDict()
	json.Unmarshal(b, result)

	// The actual secret should not be stored in clear text
	_, pres := result.Get("secret")
	assert.False(t, pres)

	data, pres := result.Get("encryptedSecret")
	assert.True(t, pres)
	assert.True(t, len(data.(string)) > 10)
}

func getSecretFromStore(
	t *testing.T,
	config_obj *config_proto.Config,
	db *datastore.MemcacheDatastore,
	path string) *ordereddict.Dict {

	path_spec := path_specs.NewUnsafeDatastorePath(strings.Split(path, "/")...)
	secret_proto := &api_proto.Secret{}
	err := db.GetSubject(config_obj, path_spec, secret_proto)
	if err != nil {
		return ordereddict.NewDict()
	}

	result, err := secrets.NewSecretFromProto(context.Background(),
		config_obj, secret_proto)
	if err != nil {
		return ordereddict.NewDict()
	}

	verifyData(t, config_obj, db, path_spec)

	return ordereddict.NewDict().
		Set("name", result.Name).
		Set("typeName", result.TypeName).
		Set("secret", result.Data).
		Set("users", result.Users)
}

func TestSecretsService(t *testing.T) {
	suite.Run(t, &SecretsTestSuite{})
}
