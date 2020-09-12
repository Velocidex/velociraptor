package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

const (
	testArtifact1 = `
name: Test.Artifact
description: This is a test artifact
parameters:
 - name: Foo
   description: A foo variable
   default: DefaultBar1

sources:
- query:  |
    SELECT * FROM info()
`

	testArtifactWithTools = `
name: Test.Artifact.Tools
tools:
 - name: Tool1
   url: http://www.google.com/tool1.exe

sources:
- query:  |
    SELECT * FROM info()
`

	testArtifactWithPermissions = `
name: Test.Artifact.Permissions
required_permissions:
- EXECVE

sources:
- query:  |
    SELECT * FROM info()
`
	testArtifactWithDeps = `
name: Test.Artifact.Deps
description: This is a test artifact dependency
sources:
- query: |
    SELECT * FROM Artifact.Test.Artifact()
`
)

type LauncherTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	sm         *services.Service
}

func (self *LauncherTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	t := self.T()
	assert.NoError(t, self.sm.Start(journal.StartJournalService))
	assert.NoError(t, self.sm.Start(notifications.StartNotificationService))
	assert.NoError(t, self.sm.Start(inventory.StartInventoryService))
	assert.NoError(t, self.sm.Start(StartLauncherService))
	assert.NoError(t, self.sm.Start(repository.StartRepositoryManager))
}

func (self *LauncherTestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

// Tools allow Velociraptor to automatically manage external bundles
// (such as external executables) and push those to the clients. The
// Artifact definition simply specified the name of the tool and where
// to fetch it from, and the server will automatically cache these and
// make the binaries available to clients.

// It is possible to either server the binaries directly from
// Velociraptor's public directory, or simply have the endpoint
// download the tool from an external location.

func (self *LauncherTestSuite) TestCompilingWithTools() {
	// Our tool binary and its hash.
	message := []byte("Hello world")
	sha_sum := sha256.New()
	sha_sum.Write(message)
	sha_value := hex.EncodeToString(sha_sum.Sum(nil))

	// Serve our tool from a test http server. First response will give 404 - not found.
	status := 404

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write(message)
	}))
	defer ts.Close()

	repository := services.GetRepositoryManager().NewRepository()
	artifact, err := repository.LoadYaml(testArtifactWithTools, true)
	assert.NoError(self.T(), err)

	// Update the tool to be downloaded from our test http instance.
	tool_url := ts.URL + "/mytool.exe"
	artifact.Tools[0].Url = tool_url

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:      "UserX",
		ClientId:     "C.1234",
		Artifacts:    []string{"Test.Artifact.Tools"},
		OpsPerSecond: 42,
		Timeout:      73,
	}
	ctx := context.Background()
	acl_manager := vql_subsystem.NullACLManager{}

	// Simulate an error downloading the tool on demand - this
	// prevents the VQL from being compiled, and therefore
	// collection can not be scheduled. The server needs to
	// download the file in order to calculate its hash - even
	// though it is not serving it to clients.
	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(ctx, self.config_obj,
		acl_manager, repository, request)
	assert.Error(self.T(), err)

	// Now make the tool download succeed. Compiling should work
	// and we should calculate the hash.
	status = 200
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, request)
	assert.NoError(self.T(), err)

	// Now that we already know the hash, we dont care about
	// downloading the file ourselves - further compiles will work
	// automatically.
	status = 404
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, request)
	assert.NoError(self.T(), err)

	// Check the compiler produced the correct environment
	// vars. When the VQL calls Generic.Utils.FetchBinary() it
	// will be able to resolve these from the environment.
	assert.Equal(self.T(), getEnvValue(compiled.Env, "Tool_Tool1_HASH"), sha_value)
	assert.Equal(self.T(), getEnvValue(compiled.Env, "Tool_Tool1_FILENAME"), "mytool.exe")
	assert.Equal(self.T(), getEnvValue(compiled.Env, "Tool_Tool1_URL"), tool_url)

	assert.Equal(self.T(), len(compiled.Query), 2)

	// Now serve the tool from Velociraptor's public directory
	// instead.
	err = services.GetInventory().AddTool(
		self.config_obj, &artifacts_proto.Tool{
			Name: "Tool1",
			// This will force Velociraptor to generate a stable
			// public directory URL from where to serve the
			// tool. The "tools upload" command will copy the
			// actual tool there.
			ServeLocally: true,
			Filename:     "mytool.exe",
			Url:          tool_url,
		}, services.ToolOptions{AdminOverride: true})
	assert.NoError(self.T(), err)

	status = 200
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, request)
	assert.NoError(self.T(), err)

	filename := paths.ObfuscateName(self.config_obj, "Tool1")

	assert.Equal(self.T(), getEnvValue(compiled.Env, "Tool_Tool1_HASH"), sha_value)
	assert.Equal(self.T(), getEnvValue(compiled.Env, "Tool_Tool1_FILENAME"), "mytool.exe")
	assert.Equal(self.T(), getEnvValue(compiled.Env, "Tool_Tool1_URL"),
		"https://localhost:8000/public/"+filename)
}

func getEnvValue(env []*actions_proto.VQLEnv, key string) string {
	for _, e := range env {
		if e.Key == key {
			return e.Value
		}
	}
	return ""
}

func (self *LauncherTestSuite) TestCompiling() {
	repository := services.GetRepositoryManager().NewRepository()
	_, err := repository.LoadYaml(testArtifact1, true)
	assert.NoError(self.T(), err)

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:   "UserX",
		ClientId:  "C.1234",
		Artifacts: []string{"Test.Artifact"},
		Parameters: &flows_proto.ArtifactParameters{
			Env: []*actions_proto.VQLEnv{
				{Key: "Foo", Value: "ParameterBar"},
			},
		},
		OpsPerSecond: 42,
		Timeout:      73,
	}
	ctx := context.Background()
	acl_manager := vql_subsystem.NullACLManager{}

	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, request)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 1, len(compiled.Env))

	serialized, err := json.Marshal(compiled.Env)
	assert.NoError(self.T(), err)

	// Should not include artifact default parameters and only
	// include provided parameters.
	assert.Equal(self.T(), string(serialized),
		"[{\"key\":\"Foo\",\"value\":\"ParameterBar\"}]")

	assert.Equal(self.T(), compiled.OpsPerSecond, request.OpsPerSecond)
	assert.Equal(self.T(), compiled.Timeout, request.Timeout)

	// Compile into 2 queries, the last have a valid Name field.
	assert.Equal(self.T(), len(compiled.Query), 2)
	assert.NotEqual(self.T(), compiled.Query[1].Name, "")
}

func (self *LauncherTestSuite) TestCompilingObfuscation() {
	repository := services.GetRepositoryManager().NewRepository()
	_, err := repository.LoadYaml(testArtifact1, true)
	assert.NoError(self.T(), err)

	self.config_obj.Frontend.DoNotCompressArtifacts = true

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:    "UserX",
		ClientId:   "C.1234",
		Artifacts:  []string{"Test.Artifact"},
		Parameters: &flows_proto.ArtifactParameters{},
	}
	ctx := context.Background()
	acl_manager := vql_subsystem.NullACLManager{}

	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, request)
	assert.NoError(self.T(), err)

	// When we do not obfuscate, artifact descriptions are carried
	// into the compiled form.
	assert.Equal(self.T(), compiled.Query[1].Description, "This is a test artifact")

	// However when we obfuscate we remove descriptions.
	self.config_obj.Frontend.DoNotCompressArtifacts = false
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, request)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), compiled.Query[1].Description, "")

}

func (self *LauncherTestSuite) TestCompilingPermissions() {
	repository := services.GetRepositoryManager().NewRepository()
	_, err := repository.LoadYaml(testArtifactWithPermissions, true)
	assert.NoError(self.T(), err)

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:      "UserX",
		ClientId:     "C.1234",
		Artifacts:    []string{"Test.Artifact.Permissions"},
		OpsPerSecond: 42,
		Timeout:      73,
	}
	ctx := context.Background()

	acl_manager := vql_subsystem.NewServerACLManager(self.config_obj, "UserX")

	// Permission denied - the principal is not allowed to compile this artifact.
	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, request)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "EXECVE")

	// Lets give the user some permissions.
	err = acls.SetPolicy(self.config_obj, "UserX",
		&acl_proto.ApiClientACL{Execve: true})
	assert.NoError(self.T(), err)

	// Should be fine now.
	acl_manager = vql_subsystem.NewServerACLManager(self.config_obj, "UserX")
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, request)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(compiled.Query), 2)
}

func TestLauncher(t *testing.T) {
	suite.Run(t, &LauncherTestSuite{})
}
