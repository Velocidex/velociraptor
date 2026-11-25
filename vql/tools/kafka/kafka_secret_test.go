package kafka

import (
	"context"
	"log"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"github.com/twmb/franz-go/pkg/kfake"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"
)

// The server side of kafka_upload(): the Kafka Creds secret type, the
// per-secret user list, and vql_must_use_secrets.
type KafkaSecretTestSuite struct {
	test_utils.TestSuite
	cluster *kfake.Cluster
}

func (self *KafkaSecretTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	var err error
	self.cluster, err = kfake.NewCluster(kfake.NumBrokers(1),
		kfake.SeedTopics(1, "vr.events"),
		kfake.EnableSASL(),
		kfake.Superuser("SCRAM-SHA-256", "vr", "s3cret"))
	assert.NoError(self.T(), err)
}

func (self *KafkaSecretTestSuite) TearDownTest() {
	self.cluster.Close()
	self.TestSuite.TearDownTest()
}

// A scope that looks enough like a server scope for the secrets
// service: a server config in the cache, and the server principal
// (NullACLManager reports VelociraptorServer).
func (self *KafkaSecretTestSuite) makeScope(logs *logBuffer) vfilter.Scope {
	scope := vql_subsystem.MakeScope().AppendPlugins(KafkaUploadPlugin{}, testRows{})
	scope.AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	vql_subsystem.CacheSet(scope, constants.SCOPE_SERVER_CONFIG, self.ConfigObj)
	scope.SetLogger(log.New(logs, "", 0))
	return scope
}

func (self *KafkaSecretTestSuite) addSecret(scope vfilter.Scope, name string, users ...string) {
	secrets_service, err := services.GetSecretsService(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = secrets_service.AddSecret(self.Ctx, scope, constants.KAFKA_CREDS, name,
		ordereddict.NewDict().
			Set("brokers", strings.Join(self.cluster.ListenAddrs(), "\n")).
			Set("sasl_mechanism", "SCRAM-SHA-256").
			Set("username", "vr").
			Set("password", "s3cret"), "admin")
	assert.NoError(self.T(), err)

	if len(users) > 0 {
		err = secrets_service.ModifySecret(self.Ctx, &api_proto.ModifySecretRequest{
			TypeName: constants.KAFKA_CREDS,
			Name:     name,
			AddUsers: users,
		}, "admin")
		assert.NoError(self.T(), err)
	}
}

func (self *KafkaSecretTestSuite) upload(scope vfilter.Scope, extra string) []*ordereddict.Dict {
	return runVQL(context.Background(), self.T(), scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=10) },
  topic="vr.events", timeout=3`+extra+`)`)
}

// The secret supplies the brokers and the SCRAM credentials; the query
// names only the secret.
func (self *KafkaSecretTestSuite) TestSecretSuppliesBrokersAndCredentials() {
	logs := &logBuffer{}
	scope := self.makeScope(logs)
	self.addSecret(scope, "shared", constants.PinnedServerName)

	sent, failed := totals(self.upload(scope, `, secret="shared"`))
	assert.Equal(self.T(), 10, sent, logs.String())
	assert.Equal(self.T(), 0, failed)
}

// A secret that has not been shared with the caller is refused before
// anything is sent.
func (self *KafkaSecretTestSuite) TestUnsharedSecretIsRefused() {
	logs := &logBuffer{}
	scope := self.makeScope(logs)
	self.addSecret(scope, "private")

	rows := self.upload(scope, `, secret="private"`)
	assert.Equal(self.T(), 0, len(rows))
	assert.Contains(self.T(), logs.String(), "Permission Denied")
}

// With vql_must_use_secrets, credentials in the query are refused.
func (self *KafkaSecretTestSuite) TestMustUseSecrets() {
	old := self.ConfigObj.Security
	self.ConfigObj.Security = &config_proto.Security{VqlMustUseSecrets: true}
	defer func() { self.ConfigObj.Security = old }()

	logs := &logBuffer{}
	scope := self.makeScope(logs)
	scope.AppendVars(ordereddict.NewDict().Set("Brokers", self.cluster.ListenAddrs()))

	rows := self.upload(scope, `, brokers=Brokers,
  sasl_mechanism="SCRAM-SHA-256", username="vr", password="s3cret"`)
	assert.Equal(self.T(), 0, len(rows))
	assert.Contains(self.T(), logs.String(), "kafka_upload:")

	// The same upload through a shared secret still works.
	self.addSecret(scope, "enforced", constants.PinnedServerName)
	sent, _ := totals(self.upload(scope, `, secret="enforced"`))
	assert.Equal(self.T(), 10, sent, logs.String())
}

// Outside the server (no server config in the scope), secret= is an
// error rather than a silent fallback to the query's arguments.
func TestSecretNeedsServer(t *testing.T) {
	logs := &logBuffer{}
	scope := newScope(nil)
	scope.SetLogger(log.New(logs, "", 0))
	runVQL(context.Background(), t, scope,
		`SELECT * FROM kafka_upload(query={SELECT 1 AS A FROM scope()}, secret="x")`)
	if !strings.Contains(logs.String(), "secrets may only be used on the server") {
		t.Fatalf("logs=%q", logs)
	}
}

func TestKafkaSecrets(t *testing.T) {
	suite.Run(t, &KafkaSecretTestSuite{})
}
