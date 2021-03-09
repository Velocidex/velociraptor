package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"

	// Load plugins (timestamp, parse_csv)
	_ "www.velocidex.com/golang/velociraptor/vql/functions"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers/csv"
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

	testArtifact2 = `
name: Test.Artifact2
parameters:
 - name: Foo
   default: Foo2

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

	testArtifactWithDepsWithTools = `
name: Test.Artifact.DepsWithTool
description: This is a test artifact dependency
sources:
- query: |
    SELECT * FROM Artifact.Test.Artifact.Tools()
`

	testArtifactWithDeps2 = `
name: Test.Artifact.Deps2
description: This is a test artifact dependency
sources:
- query: |
    SELECT * FROM Artifact.Test.Artifact.Deps()
`

	// Make sure that an artifact receives types parameters if
	// specified. The compiler should convert these appropriately
	// behind the scenes. For backwards compatibility we check
	// that use patterns in existing artifacts still work as
	// expected.
	testArtifactWithTypes = `
name: Test.Artifact.Types
parameters:
- name: IntValue
  type: int
  default: "5"

- name: CSVValue
  type: csv
  default: |
    Header
    Value1
    Value2

- name: TimestampValue
  type: timestamp

- name: TimestampValueUnset
  type: timestamp

- name: BoolValue
  type: bool
  default: Y

- name: BoolValue2
  type: bool

- name: BoolValueUnset
  type: bool

sources:
- query: |
     // Check that unset timestamps can be tested for and overriden
     // with useful defaults. bool(TimestampValueUnset) is false
     LET DateAfterTime <= if(condition=TimestampValueUnset,
        then=timestamp(epoch=TimestampValueUnset), else=timestamp(epoch="1600-01-01"))

     // TimestampValue is set so bool(TimestampValue) should be true
     LET DateBeforeTime <= if(condition=TimestampValue,
        then=timestamp(epoch=TimestampValue), else=timestamp(epoch="2200-01-01"))

     SELECT IntValue,

            // CSV parameters are parsed into an array of dicts.
            CSVValue,

            // timestamp() is passthrough for time.Time objects
            TimestampValue, TimestampValue.Unix, timestamp(epoch=TimestampValue),

            // Unset timestamps are initialized to empty time.Time
            TimestampValueUnset, TimestampValueUnset.Unix, DateAfterTime, DateBeforeTime,
            BoolValue, BoolValue2, BoolValueUnset,

            // A lot of code still compares the bool to "Y" so this
            // still needs to work - bool eq protocol uses "Y" as true.
            BoolValue = "Y", BoolValue != "Y", BoolValue2 = "Y", BoolValue2 != "Y",
            {
                SELECT * FROM CSVValue
            }
     FROM scope()
`
	testArtifactWithDepsTypes = `
name: Test.Artifact.Deps.Types
parameters:
- name: IntValue
  type: int
  default: 5

- name: CSVValue
  type: csv
  default: |
    Header
    Value1
    Value2

sources:
- query: |
    SELECT IntValue, CSVValue, BoolValue, BoolValue2
    FROM Artifact.Test.Artifact.Types(
       CSVValue=CSVValue, IntValue=IntValue,
       BoolValue=TRUE, BoolValue2="Y")
`

	testArtifactWithPrecondition = `
name: Test.Artifact.Precondition
precondition: SELECT * FROM info() WHERE FALSE
sources:
- query: |
    SELECT * FROM info()
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

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
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
		acl_manager, repository, false, request)
	assert.Error(self.T(), err)

	// Now make the tool download succeed. Compiling should work
	// and we should calculate the hash.
	status = 200
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.NoError(self.T(), err)

	// Now that we already know the hash, we dont care about
	// downloading the file ourselves - further compiles will work
	// automatically.
	status = 404
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.NoError(self.T(), err)

	// Check the compiler produced the correct environment
	// vars. When the VQL calls Generic.Utils.FetchBinary() it
	// will be able to resolve these from the environment.
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_HASH"), sha_value)
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_FILENAME"), "mytool.exe")
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_URL"), tool_url)

	assert.Equal(self.T(), len(compiled[0].Query), 2)

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
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.NoError(self.T(), err)

	filename := paths.ObfuscateName(self.config_obj, "Tool1")

	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_HASH"), sha_value)
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_FILENAME"), "mytool.exe")
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_URL"),
		"https://localhost:8000/public/"+filename)
}

func (self *LauncherTestSuite) TestGetDependentArtifacts() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(testArtifact1, true)
	assert.NoError(self.T(), err)

	_, err = repository.LoadYaml(testArtifactWithDeps, true)
	assert.NoError(self.T(), err)

	_, err = repository.LoadYaml(testArtifactWithDeps2, true)
	assert.NoError(self.T(), err)

	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	res, err := launcher.GetDependentArtifacts(self.config_obj,
		repository, []string{"Test.Artifact.Deps2"})
	assert.NoError(self.T(), err)

	sort.Strings(res)
	assert.Equal(self.T(), []string{"Test.Artifact",
		"Test.Artifact.Deps", "Test.Artifact.Deps2"}, res)
}

func (self *LauncherTestSuite) TestGetDependentArtifactsWithTool() {
	// Our tool binary and its hash.
	message := []byte("Hello world")
	sha_sum := sha256.New()
	sha_sum.Write(message)
	sha_value := hex.EncodeToString(sha_sum.Sum(nil))
	status := 200

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write(message)
	}))
	defer ts.Close()

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(testArtifactWithDepsWithTools, true)
	assert.NoError(self.T(), err)

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
		Artifacts:    []string{"Test.Artifact.DepsWithTool"},
		OpsPerSecond: 42,
		Timeout:      73,
	}
	ctx := context.Background()
	acl_manager := vql_subsystem.NullACLManager{}

	// Compile the request.
	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(ctx, self.config_obj,
		acl_manager, repository, false, request)
	assert.NoError(self.T(), err)

	// Check the compiler produced the correct environment
	// vars.

	// The environment vars of the main artifact should not have any tool info.
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_HASH"), "")
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_FILENAME"), "")
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_URL"), "")

	// The tools info should be added to the included artifacts parameters.
	artifact = compiled[0].Artifacts[0]
	assert.Equal(self.T(), getParameterValue(artifact.Parameters, "Tool_Tool1_HASH"), sha_value)
	assert.Equal(self.T(), getParameterValue(artifact.Parameters, "Tool_Tool1_FILENAME"), "mytool.exe")
	assert.Equal(self.T(), getParameterValue(artifact.Parameters, "Tool_Tool1_URL"), tool_url)

	// Dependent artifacts have no tools declared themselves.
	assert.Nil(self.T(), artifact.Tools)
}

func getEnvValue(env []*actions_proto.VQLEnv, key string) string {
	for _, e := range env {
		if e.Key == key {
			return e.Value
		}
	}
	return ""
}

func getParameterValue(params []*artifacts_proto.ArtifactParameter, key string) string {
	for _, p := range params {
		if p.Name == key {
			return p.Default
		}
	}
	return ""
}

func (self *LauncherTestSuite) TestCompiling() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(testArtifact1, true)
	assert.NoError(self.T(), err)

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:   "UserX",
		ClientId:  "C.1234",
		Artifacts: []string{"Test.Artifact"},
		Specs: []*flows_proto.ArtifactSpec{
			{
				Artifact: "Test.Artifact",
				Parameters: &flows_proto.ArtifactParameters{
					Env: []*actions_proto.VQLEnv{
						{Key: "Foo", Value: "ParameterBar"},
					},
				},
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
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 1, len(compiled[0].Env))

	serialized, err := json.Marshal(compiled[0].Env)
	assert.NoError(self.T(), err)

	// Should not include artifact default parameters and only
	// include provided parameters.
	assert.Equal(self.T(), string(serialized),
		"[{\"key\":\"Foo\",\"value\":\"ParameterBar\"}]")

	assert.Equal(self.T(), compiled[0].OpsPerSecond, request.OpsPerSecond)
	assert.Equal(self.T(), compiled[0].Timeout, request.Timeout)

	// Compile into 2 queries, the last have a valid Name field.
	assert.Equal(self.T(), len(compiled[0].Query), 2)
	assert.NotEqual(self.T(), compiled[0].Query[1].Name, "")
}

func (self *LauncherTestSuite) TestCompilingMultipleArtifacts() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(testArtifact1, true)
	assert.NoError(self.T(), err)
	_, err = repository.LoadYaml(testArtifact2, true)
	assert.NoError(self.T(), err)

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:   "UserX",
		ClientId:  "C.1234",
		Artifacts: []string{"Test.Artifact", "Test.Artifact2"},
		Specs: []*flows_proto.ArtifactSpec{
			{
				Artifact: "Test.Artifact",
				Parameters: &flows_proto.ArtifactParameters{
					Env: []*actions_proto.VQLEnv{
						{Key: "Foo", Value: "Foo1"},
					},
				},
			},
			{
				Artifact: "Test.Artifact2",
				Parameters: &flows_proto.ArtifactParameters{
					Env: []*actions_proto.VQLEnv{
						{Key: "Foo", Value: "Foo2"},
					},
				},
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
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.NoError(self.T(), err)

	// There should be two separate requests with separate values
	// for the same key.
	assert.Equal(self.T(), len(compiled), 2)
	assert.Equal(self.T(), compiled[0].Env[0].Key, "Foo")
	assert.Equal(self.T(), compiled[0].Env[0].Value, "Foo1")
	assert.Equal(self.T(), compiled[1].Env[0].Key, "Foo")
	assert.Equal(self.T(), compiled[1].Env[0].Value, "Foo2")
}

// Server events need to be compiled slighly differently - each source
// needs to run in its own goroutine.
func (self *LauncherTestSuite) TestCompilingServerEvents() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	definitions := `
name: Server.Events
type: SERVER_EVENT
parameters:
- name: Foo
  type: bool
- name: Bar
  type: bool

sources:
- name: Source1
  query: |
     SELECT Foo FROM info()

- name: Source2
  query: |
     SELECT Bar FROM info()
`

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(definitions, true)
	assert.NoError(self.T(), err)

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: []string{"Server.Events"},
		Specs: []*flows_proto.ArtifactSpec{
			{
				Artifact: "Server.Events",
				Parameters: &flows_proto.ArtifactParameters{
					Env: []*actions_proto.VQLEnv{
						{Key: "Foo", Value: "Y"},
					},
				},
			},
		},
	}

	ctx := context.Background()
	acl_manager := vql_subsystem.NullACLManager{}

	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.NoError(self.T(), err)

	// There should be 2 queries that will run in parallel.
	assert.Equal(self.T(), 2, len(compiled))

	// The parameters (Env) and type conversion preamble should be
	// duplicated across both VQLCollectorArgs instances.
	goldie.Assert(self.T(), "TestCompilingServerEvents", json.MustMarshalIndent(compiled))
}

func (self *LauncherTestSuite) TestCompilingObfuscation() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(testArtifact1, true)
	assert.NoError(self.T(), err)

	self.config_obj.Frontend.DoNotCompressArtifacts = true

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:   "UserX",
		ClientId:  "C.1234",
		Artifacts: []string{"Test.Artifact"},
	}
	ctx := context.Background()
	acl_manager := vql_subsystem.NullACLManager{}

	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.NoError(self.T(), err)

	// When we do not obfuscate, artifact descriptions are carried
	// into the compiled form.
	assert.Equal(self.T(), compiled[0].Query[1].Description, "This is a test artifact")

	// However when we obfuscate we remove descriptions.
	self.config_obj.Frontend.DoNotCompressArtifacts = false
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository,
		true, /* should_obfuscate */
		request)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), compiled[0].Query[1].Description, "")

}

func (self *LauncherTestSuite) TestCompilingPermissions() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(testArtifactWithPermissions, true)
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
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "EXECVE")

	// Lets give the user some permissions.
	err = acls.SetPolicy(self.config_obj, "UserX",
		&acl_proto.ApiClientACL{Execve: true})
	assert.NoError(self.T(), err)

	// Should be fine now.
	acl_manager = vql_subsystem.NewServerACLManager(self.config_obj, "UserX")
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(compiled[0].Query), 2)
}

func (self *LauncherTestSuite) TestParameterTypes() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(testArtifactWithTypes, true)
	assert.NoError(self.T(), err)

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:   "UserX",
		ClientId:  "C.1234",
		Artifacts: []string{"Test.Artifact.Types"},
		Specs: []*flows_proto.ArtifactSpec{
			{
				// Artifact Parameters are **always**
				// sent as strings but VQL code will
				// convert them to their correct
				// types.
				Artifact: "Test.Artifact.Types",
				Parameters: &flows_proto.ArtifactParameters{
					Env: []*actions_proto.VQLEnv{
						{Key: "IntValue", Value: "9"},
						{Key: "CSVValue",
							Value: "Col1,Col2\nValue1,Value2\nValue3,Value4\n"},
						{Key: "TimestampValue",
							Value: "2020-10-01T09:20Z"},
						{Key: "BoolValue", Value: "N"},
						{Key: "BoolValue2", Value: "Y"},
					},
				},
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Compile the artifact request into VQL
	acl_manager := vql_subsystem.NullACLManager{}
	launcher, err := services.GetLauncher()
	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.NoError(self.T(), err)

	// Now run the VQL and receive the rows back
	test_responder := responder.TestResponder()
	for _, vql_request := range compiled {
		actions.VQLClientAction{}.StartQuery(
			self.config_obj, ctx, test_responder, vql_request)
	}

	results := getResponses(test_responder)
	goldie.Assert(self.T(), "TestParameterTypes", json.MustMarshalIndent(results))
}

// Parsing of parameters only occurs at the launching artifact -
// dependent artifacts receive proper typed objects.
func (self *LauncherTestSuite) TestParameterTypesDeps() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(testArtifactWithTypes, true)
	assert.NoError(self.T(), err)

	_, err = repository.LoadYaml(testArtifactWithDepsTypes, true)
	assert.NoError(self.T(), err)

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:   "UserX",
		ClientId:  "C.1234",
		Artifacts: []string{"Test.Artifact.Deps.Types"},
		Specs: []*flows_proto.ArtifactSpec{
			{
				// Artifact Parameters are **always**
				// sent as strings but VQL code will
				// convert them to their correct
				// types.
				Artifact: "Test.Artifact.Deps.Types",
				Parameters: &flows_proto.ArtifactParameters{
					Env: []*actions_proto.VQLEnv{
						{Key: "IntValue", Value: "9"},
						{Key: "CSVValue",
							Value: "Col1,Col2\nValue1,Value2\nValue3,Value4\n"},
					},
				},
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Compile the artifact request into VQL
	acl_manager := vql_subsystem.NullACLManager{}
	launcher, err := services.GetLauncher()
	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.NoError(self.T(), err)

	// Now run the VQL and receive the rows back
	test_responder := responder.TestResponder()
	for _, vql_request := range compiled {
		actions.VQLClientAction{}.StartQuery(
			self.config_obj, ctx, test_responder, vql_request)
	}

	results := getResponses(test_responder)
	goldie.Assert(self.T(), "TestParameterTypesDeps", json.MustMarshalIndent(results))
}

func (self *LauncherTestSuite) TestParameterTypesDepsQuery() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(testArtifactWithTypes, true)
	assert.NoError(self.T(), err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	builder := services.ScopeBuilder{
		Config:     self.config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Repository: repository,
		Logger:     logging.NewPlainLogger(self.config_obj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	// Passing types parameters to artifact plugin should pass
	// them without interferance.
	queries := []string{
		"SELECT BoolValue FROM Artifact.Test.Artifact.Types(BoolValue=0)",
		"SELECT BoolValue FROM Artifact.Test.Artifact.Types(BoolValue=1)",
		"SELECT BoolValue FROM Artifact.Test.Artifact.Types(BoolValue=FALSE)",
		"SELECT BoolValue FROM Artifact.Test.Artifact.Types(BoolValue=TRUE)",
		"SELECT BoolValue FROM Artifact.Test.Artifact.Types(BoolValue='N')",
		"SELECT BoolValue FROM Artifact.Test.Artifact.Types(BoolValue='Y')",

		// Check that default parameters on artifact plugin call are properly parsed.
		"SELECT CSVValue, BoolValue FROM Artifact.Test.Artifact.Types(CSVValue=[dict(Foo=1), dict(Foo=2)])",
		"SELECT IntValue FROM Artifact.Test.Artifact.Types(IntValue=5)",
		"SELECT TimestampValue FROM Artifact.Test.Artifact.Types(TimestampValue=timestamp(epoch=1608714807))",
	}

	results := []vfilter.Row{}
	for _, query := range queries {
		vql, err := vfilter.Parse(query)
		assert.NoError(self.T(), err)

		for row := range vql.Eval(ctx, scope) {
			results = append(results, row)
		}
	}
	goldie.Assert(self.T(), "TestParameterTypesDepsQuery", json.MustMarshalIndent(results))
}

func (self *LauncherTestSuite) TestPrecondition() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(testArtifactWithPrecondition, true)
	assert.NoError(self.T(), err)

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:   "UserX",
		ClientId:  "C.1234",
		Artifacts: []string{"Test.Artifact.Precondition"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Compile the artifact request into VQL
	acl_manager := vql_subsystem.NullACLManager{}
	launcher, err := services.GetLauncher()
	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.config_obj, acl_manager, repository, false, request)
	assert.NoError(self.T(), err)

	// Now run the VQL and receive the rows back
	test_responder := responder.TestResponder()
	for _, vql_request := range compiled {
		actions.VQLClientAction{}.StartQuery(
			self.config_obj, ctx, test_responder, vql_request)
	}

	results := getResponses(test_responder)
	assert.Equal(self.T(), 0, len(results))

	// The compiled query should have an if statement with a precondition
	for _, query := range compiled[0].Query {
		if query.Name != "" {
			assert.Contains(self.T(), query.VQL, "if(then=Test_Artifact_Precondition_0_0")
		}
	}
}

func TestLauncher(t *testing.T) {
	suite.Run(t, &LauncherTestSuite{})
}
