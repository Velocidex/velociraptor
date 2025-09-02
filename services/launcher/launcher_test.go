package launcher_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/vfilter"

	// Load plugins (timestamp, parse_csv)
	_ "www.velocidex.com/golang/velociraptor/accessors/data"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	launcher_mod "www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	_ "www.velocidex.com/golang/velociraptor/vql/functions"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers/csv"
	_ "www.velocidex.com/golang/velociraptor/vql/protocols"
)

type LauncherTestSuite struct {
	test_utils.TestSuite
}

// Tools allow Velociraptor to automatically manage external bundles
// (such as external executables) and push those to the clients. The
// Artifact definition simply specifies the name of the tool and where
// to fetch it from, and the server will automatically cache these and
// make the binaries available to clients.

// It is possible to either serve the binaries directly from
// Velociraptor's public directory, or simply have the endpoint
// download the tool from an external location (like an s3 bucket).

var testArtifactWithTools = `
name: Test.Artifact.Tools
tools:
 - name: Tool1
   url: "%s"

sources:
- query:  |
    SELECT * FROM info()
`

func (self *LauncherTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.ServerArtifacts = true

	self.TestSuite.SetupTest()
}

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

	// Update the tool to be downloaded from our test http instance.
	tool_url := ts.URL + "/mytool.exe"
	repository := self.LoadArtifacts(
		fmt.Sprintf(testArtifactWithTools, tool_url))

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
	acl_manager := acl_managers.NullACLManager{}

	// Simulate an error downloading the tool on demand - this
	// prevents the VQL from being compiled, and therefore
	// collection can not be scheduled. The server needs to
	// download the file in order to calculate its hash - even
	// though it is not serving it to clients.
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(ctx, self.ConfigObj,
		acl_manager, repository, services.CompilerOptions{}, request)
	assert.Error(self.T(), err)

	// Now make the tool download succeed. Compiling should work
	// and we should calculate the hash.
	status = 200
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	// Now that we already know the hash, we dont care about
	// downloading the file ourselves - further compiles will work
	// automatically.
	status = 404
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
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
	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = inventory_service.AddTool(ctx,
		self.ConfigObj, &artifacts_proto.Tool{
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
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	filename := paths.ObfuscateName(self.ConfigObj, "Tool1")

	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_HASH"), sha_value)
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_FILENAME"), "mytool.exe")
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_URL"),
		"https://localhost:8000/public/"+filename)
}

var DependentArtifacts = []string{
	`
name: Test.Artifact
description: This is a test artifact
parameters:
 - name: Foo
   description: A foo variable
   default: DefaultBar1

sources:
- query:  |
    SELECT * FROM info()
`, `
name: Test.Artifact.Deps
description: This is a test artifact dependency
sources:
- query: |
    SELECT * FROM Artifact.Test.Artifact()
`, `
name: Test.Artifact.Deps2
description: This is a test artifact dependency
sources:
- query: |
    SELECT * FROM Artifact.Test.Artifact.Deps()
`,
}

func (self *LauncherTestSuite) TestGetDependentArtifacts() {
	repository := self.LoadArtifacts(DependentArtifacts...)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	res, err := launcher.GetDependentArtifacts(self.Ctx, self.ConfigObj,
		repository, []string{"Test.Artifact.Deps2"})
	assert.NoError(self.T(), err)

	sort.Strings(res)
	assert.Equal(self.T(), []string{"Test.Artifact",
		"Test.Artifact.Deps", "Test.Artifact.Deps2"}, res)
}

// https://github.com/Velocidex/velociraptor/issues/1287
var DependentArtifactsWithImports = []string{`
name: DependedArtifactInExport
`, `
name: Custom.TheOneWithTheExport
export: |
  // Call another artifact as part of the exported code.
  LET X <= SELECT * FROM Artifact.DependedArtifactInExport()
`, `
name: Custom.TheOneWithTheImport
imports:
  - Custom.TheOneWithTheExport
sources:
  - query: |
      SELECT * FROM X
`, `
name: Custom.CallArtifactWithImports
type: CLIENT
sources:
  - query: |
      // Call an artifact which has an import.
      select * from Artifact.Custom.TheOneWithTheImport()
`}

func (self *LauncherTestSuite) TestGetDependentArtifactsWithImports() {
	repository := self.LoadArtifacts(DependentArtifactsWithImports...)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	res, err := launcher.GetDependentArtifacts(self.Ctx, self.ConfigObj,
		repository, []string{"Custom.CallArtifactWithImports"})
	assert.NoError(self.T(), err)

	sort.Strings(res)
	assert.Equal(self.T(), []string{"Custom.CallArtifactWithImports",
		"Custom.TheOneWithTheExport", "Custom.TheOneWithTheImport",
		"DependedArtifactInExport"}, res)

	request := &flows_proto.ArtifactCollectorArgs{
		Creator:   "UserX",
		ClientId:  "C.1234",
		Artifacts: []string{"Custom.CallArtifactWithImports"},
	}
	ctx := context.Background()
	acl_manager := acl_managers.NullACLManager{}

	// Compile the request.
	compiled, err := launcher.CompileCollectorArgs(ctx, self.ConfigObj,
		acl_manager, repository, services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestGetDependentArtifactsWithImports",
		json.MustMarshalIndent(compiled))
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

	tool_url := ts.URL + "/mytool.exe"
	repository := self.LoadArtifacts(`
name: Test.Artifact.DepsWithTool
description: This is a test artifact dependency
sources:
- query: |
    SELECT * FROM Artifact.Test.Artifact.Tools()
`, fmt.Sprintf(testArtifactWithTools, tool_url))

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
	acl_manager := acl_managers.NullACLManager{}

	// Compile the request.
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(ctx, self.ConfigObj,
		acl_manager, repository, services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	// Check the compiler produced the correct environment
	// vars.

	// The environment vars of the main artifact should not have any tool info.
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_HASH"), "")
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_FILENAME"), "")
	assert.Equal(self.T(), getEnvValue(compiled[0].Env, "Tool_Tool1_URL"), "")

	// The tools info should be added to the included artifacts parameters.
	artifact := compiled[0].Artifacts[0]
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
	repository := self.LoadArtifacts(DependentArtifacts...)

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
	acl_manager := acl_managers.NullACLManager{}

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
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

var CompilingMultipleArtifacts = []string{
	`
name: Test.Artifact
description: This is a test artifact
parameters:
 - name: Foo
   description: A foo variable
   default: DefaultBar1

sources:
- query:  |
    SELECT * FROM info()
`, `
name: Test.Artifact2
parameters:
 - name: Foo
   default: Foo2

sources:
- query:  |
    SELECT * FROM info()
`, `
name: Test.ArtifactResources
resources:
 timeout: 250
 cpu_limit: 20
 max_batch_rows: 256
 max_batch_wait: 101

parameters:
 - name: Foo
   default: Foo2

sources:
- query:  |
    SELECT * FROM info()
`}

func (self *LauncherTestSuite) TestCompilingMultipleArtifacts() {
	repository := self.LoadArtifacts(CompilingMultipleArtifacts...)

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
	acl_manager := acl_managers.NullACLManager{}

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	// There should be two separate requests with separate values
	// for the same key.
	assert.Equal(self.T(), len(compiled), 2)
	assert.Equal(self.T(), compiled[0].Env[0].Key, "Foo")
	assert.Equal(self.T(), compiled[0].Env[0].Value, "Foo1")
	assert.Equal(self.T(), compiled[1].Env[0].Key, "Foo")
	assert.Equal(self.T(), compiled[1].Env[0].Value, "Foo2")
}

func (self *LauncherTestSuite) TestCompilingMultipleLimitedArtifacts() {
	repository := self.LoadArtifacts(CompilingMultipleArtifacts...)

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:   "UserX",
		ClientId:  "C.1234",
		Artifacts: []string{"Test.Artifact", "Test.ArtifactResources"},
		Specs: []*flows_proto.ArtifactSpec{
			{
				// Here we specify limits in the artifact spec.
				Artifact: "Test.Artifact",
				Parameters: &flows_proto.ArtifactParameters{
					Env: []*actions_proto.VQLEnv{
						{Key: "Foo", Value: "Foo1"},
					},
				},
				CpuLimit:           12,
				MaxBatchRows:       200,
				MaxBatchRowsBuffer: 300,
				MaxBatchWait:       400,
				Timeout:            500,
			},
			{
				// This one specified limits in the artifact
				// definition.
				Artifact: "Test.ArtifactResources",
				Parameters: &flows_proto.ArtifactParameters{
					Env: []*actions_proto.VQLEnv{
						{Key: "Foo", Value: "Foo2"},
					},
				},
			},
		},
	}
	ctx := context.Background()
	acl_manager := acl_managers.NullACLManager{}

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	json.Dump(compiled)
}

// Server events need to be compiled slighly differently - each source
// needs to run in its own goroutine.
func (self *LauncherTestSuite) TestCompilingServerEvents() {
	definitions := []string{`
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
`}

	repository := self.LoadArtifacts(definitions...)

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
	acl_manager := acl_managers.NullACLManager{}

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	// There should be 2 queries that will run in parallel.
	assert.Equal(self.T(), 2, len(compiled))

	// The parameters (Env) and type conversion preamble should be
	// duplicated across both VQLCollectorArgs instances.
	goldie.Assert(self.T(), "TestCompilingServerEvents", json.MustMarshalIndent(compiled))
}

func (self *LauncherTestSuite) TestCompilingObfuscation() {
	repository := self.LoadArtifacts(CompilingMultipleArtifacts...)
	self.ConfigObj.Frontend.DoNotCompressArtifacts = true

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:   "UserX",
		ClientId:  "C.1234",
		Artifacts: []string{"Test.Artifact"},
	}
	ctx := context.Background()
	acl_manager := acl_managers.NullACLManager{}

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{
			ObfuscateNames: false,
		}, request)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), compiled[0].Query[1].Name, "Test.Artifact")
}

func (self *LauncherTestSuite) TestCompilingPermissions() {
	repository := self.LoadArtifacts(`
name: Test.Artifact.Permissions
required_permissions:
- EXECVE

sources:
- query:  |
    SELECT * FROM info()
`)

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

	acl_manager := acl_managers.NewServerACLManager(self.ConfigObj, "UserX")

	// Lets give the user some permissions.
	err := services.SetPolicy(self.ConfigObj, "UserX",
		&acl_proto.ApiClientACL{CollectClient: true})
	assert.NoError(self.T(), err)

	// Permission denied - the principal is not allowed to compile
	// this artifact.
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "EXECVE")

	// Lets give the user some permissions.
	err = services.SetPolicy(self.ConfigObj, "UserX",
		&acl_proto.ApiClientACL{Execve: true, CollectClient: true})
	assert.NoError(self.T(), err)

	// Should be fine now.
	acl_manager = acl_managers.NewServerACLManager(self.ConfigObj, "UserX")
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(compiled[0].Query), 2)
}

func (self *LauncherTestSuite) TestBasicPermissions() {
	repository := self.LoadArtifacts(`
name: Test.Artifact.BasicPermissions
sources:
- query:  |
    SELECT * FROM info()
`)

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Creator:      "UserX",
		ClientId:     "C.1234",
		Artifacts:    []string{"Test.Artifact.BasicPermissions"},
		OpsPerSecond: 42,
		Timeout:      73,
	}

	// acl_manager caches tokens so we need a new one each time.
	acl_manager := acl_managers.NewServerACLManager(
		self.ConfigObj, "UserX")

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		self.Ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.Error(self.T(), err)
	assert.True(self.T(), errors.Is(err, acls.PermissionDenied))

	// Lets give the user COLLECT_BASIC
	err = services.SetPolicy(self.ConfigObj, "UserX",
		&acl_proto.ApiClientACL{CollectBasic: true})
	assert.NoError(self.T(), err)

	// Try again - this is not enough though because the artifact is
	// not marked as "basic"
	compiled, err = launcher.CompileCollectorArgs(
		self.Ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.Error(self.T(), err)
	assert.True(self.T(), errors.Is(err, acls.PermissionDenied))

	// Mark the artifact as "Basic"
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = manager.SetArtifactMetadata(self.Ctx, self.ConfigObj,
		"UserX", "Test.Artifact.BasicPermissions",
		&artifacts_proto.ArtifactMetadata{
			Basic: true,
		})
	assert.NoError(self.T(), err)

	// Should be fine now.
	acl_manager = acl_managers.NewServerACLManager(self.ConfigObj, "UserX")
	compiled, err = launcher.CompileCollectorArgs(
		self.Ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(compiled[0].Query), 2)
}

func (self *LauncherTestSuite) TestParameterTypes() {
	repository := self.LoadArtifacts(testArtifactWithTypes...)

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
	acl_manager := acl_managers.NullACLManager{}
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	// Now run the VQL and receive the rows back
	test_responder := responder.TestResponderWithFlowId(
		self.ConfigObj, "F.TestParameterTypes")
	for _, vql_request := range compiled {
		actions.VQLClientAction{}.StartQuery(
			self.ConfigObj, ctx, test_responder, vql_request)
	}

	var messages []*ordereddict.Dict
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		messages = getResponses(test_responder.Drain.Messages())
		return len(messages) > 0
	})

	goldie.Assert(self.T(), "TestParameterTypes", json.MustMarshalIndent(messages))
}

var (
	// Make sure that an artifact receives types parameters if
	// specified. The compiler should convert these appropriately
	// behind the scenes. For backwards compatibility we check
	// that use patterns in existing artifacts still work as
	// expected.
	testArtifactWithTypes = []string{`
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
`, `
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
`}
)

// Parsing of parameters only occurs at the launching artifact -
// dependent artifacts receive proper typed objects.
func (self *LauncherTestSuite) TestParameterTypesDeps() {
	repository := self.LoadArtifacts(testArtifactWithTypes...)

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
	acl_manager := acl_managers.NullACLManager{}
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	// Now run the VQL and receive the rows back
	test_responder := responder.TestResponderWithFlowId(
		self.ConfigObj, "F.TestParameterTypesDeps")
	for _, vql_request := range compiled {
		actions.VQLClientAction{}.StartQuery(
			self.ConfigObj, ctx, test_responder, vql_request)
	}

	var messages []*ordereddict.Dict
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		messages = getResponses(test_responder.Drain.Messages())
		return len(messages) > 0
	})

	goldie.Assert(self.T(), "TestParameterTypesDeps",
		json.MustMarshalIndent(messages))
}

func (self *LauncherTestSuite) TestParameterTypesDepsQuery() {
	repository := self.LoadArtifacts(testArtifactWithTypes...)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Repository: repository,
		Logger: logging.NewPlainLogger(
			self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)
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

/*
When the precondition is at the top level, there will be a single

	request with multiple sources in the same request: Serial Mode
*/
func (self *LauncherTestSuite) TestPreconditionTopLevel() {
	repository := self.LoadArtifacts(`
name: Test.Artifact.Precondition
precondition: |
   SELECT * FROM info() WHERE FALSE
sources:
- name: Source1
  query: |
    SELECT 1 AS A FROM scope()
- name: Source2
  query: |
    SELECT 2 AS A FROM scope()
`)
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
	acl_manager := acl_managers.NullACLManager{}
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	fixture := ordereddict.NewDict().Set("WithPreconditions", compiled)

	// Compile again, this time disabling the precondition
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{
			DisablePrecondition: true,
		}, request)
	assert.NoError(self.T(), err)

	fixture.Set("WithoutPreconditions", compiled)
	goldie.Assert(self.T(), "TestPreconditionTopLevel",
		json.MustMarshalIndent(fixture))
}

/*
When preconditions are at the source level, artifact is collected

	in parallel mode.
*/
func (self *LauncherTestSuite) TestPreconditionSourceLevel() {
	repository := self.LoadArtifacts(`
name: Test.Artifact.Precondition
sources:
- name: Source1
  query: |
    SELECT 1 AS A FROM scope()
- name: Source2
  precondition: |
     SELECT * FROM info() WHERE FALSE
  query: |
    SELECT 2 AS A FROM scope()
`)
	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: []string{"Test.Artifact.Precondition"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Compile the artifact request into VQL
	acl_manager := acl_managers.NullACLManager{}
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	fixture := ordereddict.NewDict().Set("WithPreconditions", compiled)

	// Compile again, this time disabling the precondition
	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{
			DisablePrecondition: true,
		}, request)
	assert.NoError(self.T(), err)

	fixture.Set("WithoutPreconditions", compiled)
	goldie.Assert(self.T(), "TestPreconditionSourceLevel",
		json.MustMarshalIndent(fixture))
}

// Preconditions called recursively
func (self *LauncherTestSuite) TestPreconditionRecursive() {
	repository := self.LoadArtifacts(`
name: Test.Artifact.Precondition
sources:
- query: |
    select * from Artifact.MultiSourceSerialMode(preconditions=TRUE)
`, `
name: MultiSourceSerialMode
sources:
- name: Source1
  precondition: "SELECT * FROM info() WHERE FALSE"
  query: SELECT "A" AS Col FROM scope()

- name: Source2
  precondition: |
     SELECT * FROM info() WHERE TRUE
  query: |
    SELECT "B" AS Col FROM scope()
`)

	// The artifact compiler converts artifacts into a VQL request
	// to be run by the clients.
	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: []string{"Test.Artifact.Precondition"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Compile the artifact request into VQL
	acl_manager := acl_managers.NullACLManager{}
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	// Make sure the compiled artifacts also contain the precondition
	// so the client can do the right thing with them.
	assert.Equal(self.T(), compiled[0].Artifacts[0].Name, "MultiSourceSerialMode")
	assert.Equal(self.T(), compiled[0].Artifacts[0].Sources[0].Name,
		"Source1")
	assert.Equal(self.T(), compiled[0].Artifacts[0].Sources[0].Precondition,
		"SELECT * FROM info() WHERE FALSE")

	fixture := ordereddict.NewDict().Set("CompiledRequest", compiled)
	goldie.Assert(self.T(), "TestPreconditionRecursive",
		json.MustMarshalIndent(fixture))
}

// Test that compiler rejects invalid artifacts
func (self *LauncherTestSuite) TestInvalidArtifacts() {
	artifact_definitions := []string{`
name: Test.Artifact.InvalidSource
sources:
- query: |
    SELECT * FROM scope()
    LET X = SELECT * FROM scope()
`, `
name: Test.Artifact.InvalidSource2
sources:
- query: |
    SELECT * FROM scope()
    SELECT * FROM scope()
`, `
name: Test.Artifact.Precondition
description: Invalid artifact with precondition at top level and source level.
precondition: SELECT * FROM info() WHERE OS = "linux"
sources:
- name: Source1
  query: |
    SELECT 1 AS A FROM scope()
- name: Source2
  precondition: |
     SELECT * FROM info() WHERE FALSE
  query: |
    SELECT 2 AS A FROM scope()
`, `
name: Test.Artifact.SyntaxError
precondition: SELECT 1 FORM
sources:
- name: Source1
  query: |
    SELECT 1 AS A FROM scope()
`, `
name: Test.Artifact.SyntaxError
sources:
- query: |
    SELECT 1 FORM
`}

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository := manager.NewRepository()

	for _, definition := range artifact_definitions {
		_, err := repository.LoadYaml(definition,
			services.ArtifactOptions{
				ValidateArtifact:  true,
				ArtifactIsBuiltIn: true})

		assert.Error(self.T(), err, "Failed to reject "+definition)
	}

}

func (self *LauncherTestSuite) TestArtifactResources() {
	artifact_definitions := []string{`
name: Test.Artifact.Timeout
resources:
  timeout: 5
  max_rows: 10
sources:
- query: |
    SELECT * FROM scope()
`, `
name: Test.Artifact.MaxRows
resources:
  max_rows: 20
sources:
- query: |
    SELECT * FROM scope()
`}

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository := manager.NewRepository()

	for _, definition := range artifact_definitions {
		_, err := repository.LoadYaml(definition,
			services.ArtifactOptions{
				ValidateArtifact:  true,
				ArtifactIsBuiltIn: true})

		assert.NoError(self.T(), err)
	}

	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: []string{
			"Test.Artifact.Timeout",
			"Test.Artifact.MaxRows",
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Compile the artifact request into VQL
	acl_manager := acl_managers.NullACLManager{}
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	// No timeout specified in the request causes the timeout to
	// be set according to the artifact defaults.
	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), getReqName(compiled[0]), "Test.Artifact.Timeout")
	assert.Equal(self.T(), compiled[0].Timeout, uint64(5))

	// Timeout is not specified in the artifact so it will take on
	// default value.
	assert.Equal(self.T(), getReqName(compiled[1]), "Test.Artifact.MaxRows")
	assert.Equal(self.T(), compiled[1].Timeout, uint64(0))

	// MaxRows is enforced by the server on the entire collection,
	// therefore the highest MaxRows in any of the collected
	// artifacts will be chosen.
	assert.Equal(self.T(), request.MaxRows, uint64(20))

	// Specifying timeout in the request overrides all defaults.
	request.Timeout = 20
	request.MaxRows = 100
	request.ProgressTimeout = 21

	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), getReqName(compiled[0]), "Test.Artifact.Timeout")
	assert.Equal(self.T(), compiled[0].Timeout, uint64(5))

	assert.Equal(self.T(), getReqName(compiled[1]), "Test.Artifact.MaxRows")
	assert.Equal(self.T(), compiled[1].Timeout, uint64(20))
	assert.Equal(self.T(), compiled[1].ProgressTimeout, float32(21))

	// Specifying MaxRows in the request overrides the setting.
	assert.Equal(self.T(), request.MaxRows, uint64(100))
}

func (self *LauncherTestSuite) TestMaxWait() {
	artifact_definitions := []string{`
name: Test.Artifact.MaxWait
resources:
  max_batch_wait: 22
  max_batch_rows: 555

type: CLIENT_EVENT

sources:
- query: |
    SELECT * FROM scope()
`, `
name: Test.Artifact.Default
type: CLIENT_EVENT
sources:
- query: |
    SELECT * FROM scope()
`}

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository := manager.NewRepository()

	for _, definition := range artifact_definitions {
		_, err := repository.LoadYaml(definition,
			services.ArtifactOptions{
				ValidateArtifact:  true,
				ArtifactIsBuiltIn: true})

		assert.NoError(self.T(), err)
	}

	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: []string{
			"Test.Artifact.MaxWait",
			"Test.Artifact.Default",
		},
		Specs: []*flows_proto.ArtifactSpec{
			{
				Artifact: "Test.Artifact.MaxWait",
			},
			{
				Artifact: "Test.Artifact.Default",
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Compile the artifact request into VQL
	acl_manager := acl_managers.NullACLManager{}
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	// No timeout specified in the request causes the timeout to
	// be set according to the artifact defaults.
	compiled, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	// This artifact default is specified as MaxBatchRows = 555
	assert.Equal(self.T(), getReqName(compiled[0]), "Test.Artifact.MaxWait")
	assert.Equal(self.T(), compiled[0].MaxRow, uint64(555))
	assert.Equal(self.T(), compiled[0].MaxWait, uint64(22))

	// This artifact is not specified so MaxBatchRows is the default
	// 1000
	assert.Equal(self.T(), getReqName(compiled[1]), "Test.Artifact.Default")
	assert.Equal(self.T(), compiled[1].MaxRow, uint64(1000))

	// Specifying MaxBatchRows in the spec will override the default
	request.Specs[0].MaxBatchRows = 12
	request.Specs[0].MaxBatchWait = 66

	compiled, err = launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	// The spec defines the Test.Artifact.MaxWait should have MaxRow = 12
	assert.Equal(self.T(), getReqName(compiled[0]), "Test.Artifact.MaxWait")
	assert.Equal(self.T(), compiled[0].MaxRow, uint64(12))
	assert.Equal(self.T(), compiled[0].MaxWait, uint64(66))

	// Does not affect the Test.Artifact.Default request.
	assert.Equal(self.T(), getReqName(compiled[1]), "Test.Artifact.Default")
	assert.Equal(self.T(), compiled[1].MaxRow, uint64(1000))
}

func getReqName(in *actions_proto.VQLCollectorArgs) string {
	for _, query := range in.Query {
		if query.Name != "" {
			return query.Name
		}
	}
	return ""
}

func (self *LauncherTestSuite) TestDelete() {
	assert.Retry(self.T(), 10, time.Second, self._TestDelete)
}

func (self *LauncherTestSuite) _TestDelete(t *assert.R) {
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(t, err)

	flow_id := "F.FlowId123"
	user := "admin"

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)
	acl_manager := acl_managers.NullACLManager{}

	defer utils.SetFlowIdForTests(flow_id)()

	res, err := launcher.GetFlows(self.Ctx, self.ConfigObj, "server",
		result_sets.ResultSetOptions{}, 0, 10)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(res.Items))

	// Schedule a job for the server runner.
	flow_id, err = launcher.ScheduleArtifactCollection(
		self.Ctx, self.ConfigObj, acl_manager,
		repository, &flows_proto.ArtifactCollectorArgs{
			Creator:   user,
			ClientId:  "server",
			Artifacts: []string{"Generic.Client.Info"},
		}, utils.SyncCompleter)

	assert.NoError(t, err)

	res, err = launcher.GetFlows(self.Ctx, self.ConfigObj, "server",
		result_sets.ResultSetOptions{}, 0, 10)
	assert.NoError(t, err)
	assert.Equal(t, len(res.Items), 1)
	assert.Equal(t, res.Items[0].SessionId, flow_id)

	// Now delete the flow asyncronously
	_, err = launcher.Storage().DeleteFlow(
		self.Ctx, self.ConfigObj, "server",
		flow_id, constants.PinnedServerName,
		services.DeleteFlowOptions{
			ReallyDoIt: true,
			Sync:       false,
		})
	assert.NoError(t, err)

	// Index is not updated yet
	idx := self.getIndex("server")
	assert.Equal(t, len(idx), 1)
	idx_flow_id, _ := idx[0].GetString("FlowId")
	assert.Equal(t, flow_id, idx_flow_id)

	datastore.FlushDatastore(self.ConfigObj)

	// However GetFlows omits the deleted flow immediately because it
	// can not find it (The actual flow object is removed but the
	// index is out of step).
	vtesting.WaitUntil(10*time.Second, t, func() bool {
		// Force the housekeep thread to run immediately.
		launcher.Storage().(*launcher_mod.FlowStorageManager).
			RemoveFlowsFromJournal(self.Ctx, self.ConfigObj)

		datastore.FlushDatastore(self.ConfigObj)

		res, err = launcher.GetFlows(self.Ctx, self.ConfigObj, "server",
			result_sets.ResultSetOptions{}, 0, 10)
		assert.NoError(t, err)
		time.Sleep(time.Second)
		return len(res.Items) == 0
	})
	assert.Equal(t, len(res.Items), 0)

	// Create the flow again
	new_flow_id, err := launcher.ScheduleArtifactCollection(
		self.Ctx, self.ConfigObj, acl_manager,
		repository, &flows_proto.ArtifactCollectorArgs{
			Creator:   user,
			ClientId:  "server",
			Artifacts: []string{"Generic.Client.Info"},
		}, utils.SyncCompleter)
	assert.NoError(t, err)
	assert.Equal(t, new_flow_id, flow_id)

	// Now delete the flow syncronously
	_, err = launcher.Storage().DeleteFlow(
		self.Ctx, self.ConfigObj, "server",
		flow_id, constants.PinnedServerName,
		services.DeleteFlowOptions{
			ReallyDoIt: true,
			Sync:       true,
		})
	assert.NoError(t, err)

	datastore.FlushDatastore(self.ConfigObj)

	// This time the index is reset immediately.
	idx = self.getIndex("server")
	assert.Equal(t, len(idx), 0)
}

func (self *LauncherTestSuite) getIndex(client_id string) (
	res []*ordereddict.Dict) {

	client_path_manager := paths.NewClientPathManager(client_id)
	file_store_factory := file_store.GetFileStore(self.ConfigObj)
	rs_reader, err := result_sets.NewResultSetReader(file_store_factory,
		client_path_manager.FlowIndex())
	if err != nil {
		return nil
	}
	defer rs_reader.Close()

	for r := range rs_reader.Rows(self.Ctx) {
		res = append(res, r)
	}
	return res
}

func TestLauncher(t *testing.T) {
	suite.Run(t, &LauncherTestSuite{})
}
