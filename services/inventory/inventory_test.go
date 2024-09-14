package inventory_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type MockClient struct {
	responses map[string]string
	requests  []string

	count int
}

func (self *MockClient) Do(req *http.Request) (*http.Response, error) {
	url := req.URL.String()

	response := self.responses[url]
	self.requests = append(self.requests, url)
	self.count++

	return &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewReader([]byte(response))),
	}, nil
}

type ServicesTestSuite struct {
	test_utils.TestSuite
	client_id string
	flow_id   string
	mock      *MockClient
}

func (self *ServicesTestSuite) TestGihubTools() {
	ctx := context.Background()
	tool_name := "SampleTool"
	golden := ordereddict.NewDict()

	self.installGitHubMock()

	// Add a new tool from github.
	inventory, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = inventory.AddTool(ctx,
		self.ConfigObj, &artifacts_proto.Tool{
			Name:             tool_name,
			GithubProject:    "Velocidex/velociraptor",
			GithubAssetRegex: "windows-amd64.exe",
		}, services.ToolOptions{
			ArtifactDefinition: true,
		})
	assert.NoError(self.T(), err)

	// Adding the tool does not result in any HTTP requests!
	assert.Equal(self.T(), len(self.mock.requests), 0)

	// Materialize the tool now - this will force HTTP requests
	tool, err := inventory.GetToolInfo(ctx, self.ConfigObj, tool_name, "")
	assert.NoError(self.T(), err)

	// Both URLs were fetched
	assert.Equal(self.T(), len(self.mock.requests), 2)

	assert.Equal(self.T(), tool.Name, "SampleTool")
	assert.Equal(self.T(), tool.Url, "htttp://www.example.com/file.exe")

	golden.Set("Tool", tool)

	// What does the launcher do?
	request := &actions_proto.VQLCollectorArgs{}
	err = launcher.AddToolDependency(
		ctx, self.ConfigObj, tool_name, "", request)
	assert.NoError(self.T(), err)

	golden.Set("VQLCollectorArgs", request)

	serialized, err := json.MarshalIndentNormalized(golden)
	assert.NoError(self.T(), err)
	goldie.Assert(self.T(), "TestGihubTools", serialized)
}

// Install a mock on the HTTP client to check the Github API for
// release assets.
func (self *ServicesTestSuite) installGitHubMock() {
	api_reply := `{"assets":[{"name":"Velociraptor-Vx.x.x-windows-amd64.exe","browser_download_url":"htttp://www.example.com/file.exe"}]}`

	self.mock = &MockClient{
		responses: map[string]string{
			"htttp://www.example.com/file.exe":                                    "File Content",
			"https://api.github.com/repos/Velocidex/velociraptor/releases/latest": api_reply,
		},
	}

	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	inventory_service.(*inventory.InventoryService).Client = self.mock
}

func (self *ServicesTestSuite) installGitHubMockVersion2() {
	// The latest release is now version 2.
	api_reply := `{"assets":[{"name":"Velociraptor-V2.x.x-windows-amd64.exe","browser_download_url":"htttp://www.example.com/file_v2.exe"}]}`

	self.mock = &MockClient{
		responses: map[string]string{
			"htttp://www.example.com/file.exe":                                    "File Content V2",
			"https://api.github.com/repos/Velocidex/velociraptor/releases/latest": api_reply,
		},
	}

	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	inventory_service.(*inventory.InventoryService).Client = self.mock
}

// Test that an artifact can add its own tools.
func (self *ServicesTestSuite) TestGihubToolsUninitialized() {
	ctx := context.Background()

	// Define a new artifact that requires a new tool
	test_artifact := `
name: TestArtifact
tools:
- name: SampleTool
  github_project: Velocidex/velociraptor
  github_asset_regex: windows-amd64.exe
`
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(test_artifact,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: true})
	assert.NoError(self.T(), err)

	self.installGitHubMock()

	// Launch the artifact - this will result in the tool being
	// downloaded and the hash calculated on demand.
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	response, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_managers.NullACLManager{}, repository,
		services.CompilerOptions{},
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
		})
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(response))

	// What is the tool info - should have resolved the final
	// destination and the hash.
	tool_name := "SampleTool"
	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	tool, err := inventory_service.GetToolInfo(ctx, self.ConfigObj, tool_name, "")
	assert.NoError(self.T(), err)

	// Make sure the tool contains the version block
	assert.Equal(self.T(), 1, len(tool.Versions))
	assert.Equal(self.T(), "TestArtifact", tool.Versions[0].Artifact)

	// Make sure the tool is served directly from upstream.
	assert.Equal(self.T(), response[0].Env[2].Key, "Tool_SampleTool_URL")
	assert.Equal(self.T(), response[0].Env[2].Value, "htttp://www.example.com/file.exe")

	assert.Equal(self.T(), response[0].Env[0].Key, "Tool_SampleTool_HASH")
	assert.Equal(self.T(), response[0].Env[0].Value,
		"3c03cf5341a1e078c438f31852e1587a70cc9f91ee02eda315dd231aba0a0ab1")

	golden := ordereddict.NewDict().Set("Tool", tool).Set("Request", response[0])
	serialized, err := json.MarshalIndentNormalized(golden)
	assert.NoError(self.T(), err)
	goldie.Assert(self.T(), "TestGihubToolsUninitialized", serialized)
}

func (self *ServicesTestSuite) TestExpectedHash() {
	ctx := context.Background()

	// Define a new artifact that requires a new tool
	test_artifact := `
name: TestArtifact
tools:
- name: SampleTool
  github_project: Velocidex/velociraptor
  github_asset_regex: windows-amd64.exe
  expected_hash: 001122
`
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(test_artifact,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: false})
	assert.NoError(self.T(), err)

	self.installGitHubMock()

	hash := getHash("File Content")

	// Launch the artifact - this will result in the tool being
	// downloaded and the hash calculated on demand.
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Should fail because the tool hash is not the same as the
	// expected hash.
	_, err = launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_managers.NullACLManager{}, repository,
		services.CompilerOptions{},
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
		})
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "Downloaded tool hash of "+hash)

	// Now update the tool definition to trust the hash.
	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Check the tool definition in the inventory.
	tool, err := inventory_service.ProbeToolInfo(
		ctx, self.ConfigObj, "SampleTool", "")
	assert.NoError(self.T(), err)

	// Make sure the invalid hash is recorded.
	assert.Equal(self.T(), hash, tool.InvalidHash)

	// Now update the tool definition by accepting the hash
	tool.ExpectedHash = tool.InvalidHash
	err = inventory_service.AddTool(ctx, self.ConfigObj,
		tool, services.ToolOptions{
			AdminOverride: true,
		})
	assert.NoError(self.T(), err)

	// Force the tool to download again.
	tool, err = inventory_service.GetToolInfo(ctx, self.ConfigObj, "SampleTool", "")
	assert.NoError(self.T(), err)
	goldie.AssertJson(self.T(), "TestExpectedHash", tool)
}

// Test that a tool can be upgraded.
func (self *ServicesTestSuite) TestUpgrade() {
	ctx := context.Background()
	tool_name := "SampleTool"

	self.installGitHubMock()

	// Add a new tool from github.
	tool_definition := &artifacts_proto.Tool{
		Name:             tool_name,
		GithubProject:    "Velocidex/velociraptor",
		GithubAssetRegex: "windows-amd64.exe",
	}

	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = inventory_service.AddTool(ctx, self.ConfigObj,
		tool_definition, services.ToolOptions{})
	assert.NoError(self.T(), err)

	tool, err := inventory_service.GetToolInfo(ctx, self.ConfigObj, tool_name, "")
	assert.NoError(self.T(), err)

	// First version.
	assert.Equal(self.T(), tool.Url, "htttp://www.example.com/file.exe")
	assert.Equal(self.T(), tool.Hash, "3c03cf5341a1e078c438f31852e1587a70cc9f91ee02eda315dd231aba0a0ab1")

	// Now force the tool to update by re-adding it but this time it is a new version.
	self.installGitHubMockVersion2()

	err = inventory_service.AddTool(ctx, self.ConfigObj, tool_definition,
		services.ToolOptions{})
	assert.NoError(self.T(), err)

	// Check the tool information.
	tool, err = inventory_service.GetToolInfo(ctx, self.ConfigObj, tool_name, "")
	assert.NoError(self.T(), err)

	// Make sure the tool is updated and the hash is changed.
	assert.Equal(self.T(), tool.Url, "htttp://www.example.com/file_v2.exe")
	assert.Equal(self.T(), tool.Hash, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
}

// Self serve the binary
func (self *ServicesTestSuite) TestSelfServe() {
	ctx := context.Background()

	// Define a new artifact that requires a new tool
	test_artifact := `
name: TestArtifact
tools:
- name: SampleTool
  github_project: Velocidex/velociraptor
  github_asset_regex: windows-amd64.exe
  serve_locally: true
`
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	_, err = repository.LoadYaml(test_artifact,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: true})
	assert.NoError(self.T(), err)

	self.installGitHubMock()
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	response, err := launcher.CompileCollectorArgs(
		ctx, self.ConfigObj, acl_managers.NullACLManager{}, repository,
		services.CompilerOptions{},
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
		})
	assert.NoError(self.T(), err)

	// Make sure the tool is served directly from the public directory.
	assert.Equal(self.T(), response[0].Env[2].Key, "Tool_SampleTool_URL")
	assert.Contains(self.T(), response[0].Env[2].Value, "https://localhost:8000/")

	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	tool, err := inventory_service.GetToolInfo(ctx, self.ConfigObj, "SampleTool", "")
	assert.NoError(self.T(), err)

	golden := ordereddict.NewDict().Set("Tool", tool).Set("Request", response[0])
	serialized, err := json.MarshalIndentNormalized(golden)
	assert.NoError(self.T(), err)
	goldie.Assert(self.T(), "TestGihubToolServedLocally", serialized)
}

// When artifacts are parsed, they add their tool definition to the
// inventory automatically if they are not already present. This test
// makes sure that tools are added to the inventory **only** if they
// do not already exist **or** their new definition is more detailed
// than the old one.
func (self *ServicesTestSuite) TestUpgradePriority() {
	// Define a new artifact that requires a new tool
	test_artifact := `
name: TestArtifact
tools:
- name: SampleTool
  url: http://www.example.com/
`
	test_artifact2 := `
name: TestArtifact2
tools:
- name: SampleTool
`
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, _ := manager.GetGlobalRepository(self.ConfigObj)
	_, err = repository.LoadYaml(test_artifact,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: true})
	assert.NoError(self.T(), err)

	ctx := self.Ctx
	_, pres := repository.Get(ctx, self.ConfigObj, "TestArtifact")
	assert.True(self.T(), pres)

	_, err = repository.LoadYaml(test_artifact2,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: true})

	assert.NoError(self.T(), err)

	_, pres = repository.Get(ctx, self.ConfigObj, "TestArtifact2")
	assert.True(self.T(), pres)

	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	tool, err := inventory_service.ProbeToolInfo(
		ctx, self.ConfigObj, "SampleTool", "")
	assert.NoError(self.T(), err)

	// The tool definition retains the original URL
	assert.Equal(self.T(), tool.Url, "http://www.example.com/")
}

// Make sure that loading an artifact does not upgrade an admin
// override.
func (self *ServicesTestSuite) TestUpgradeAdminOverride() {
	// Define a new artifact that requires a new tool
	test_artifact := `
name: TestArtifact
tools:
- name: SampleTool
  url: http://www.example.com/
  serve_locally: true
`

	// The admin sets a very minimal tool definition.
	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = inventory_service.AddTool(self.Ctx, self.ConfigObj,
		&artifacts_proto.Tool{
			Name: "SampleTool",
			Hash: "XXXXX",
		}, services.ToolOptions{AdminOverride: true})
	assert.NoError(self.T(), err)

	// Parsing the artifact does not update the tool - admins can
	// pin the tool definition.
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, _ := manager.GetGlobalRepository(self.ConfigObj)
	_, err = repository.LoadYaml(test_artifact,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: true})

	assert.NoError(self.T(), err)

	_, pres := repository.Get(self.Ctx, self.ConfigObj, "TestArtifact")
	assert.True(self.T(), pres)

	tool, err := inventory_service.ProbeToolInfo(
		self.Ctx, self.ConfigObj, "SampleTool", "")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), tool.Url, "")
	assert.Equal(self.T(), tool.Hash, "XXXXX")
	assert.False(self.T(), tool.ServeLocally)
}

// If an admin overrides an automatically inserted tool definition,
// they should be able to.
func (self *ServicesTestSuite) TestAdminOverrideUpgrade() {
	// Define a new artifact that requires a new tool
	test_artifact := `
name: TestArtifact
tools:
- name: SampleTool
  url: http://www.example.com/
  serve_locally: true
`
	// Parsing the artifact should insert the tool.
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, _ := manager.GetGlobalRepository(self.ConfigObj)
	_, err = repository.LoadYaml(test_artifact, services.ArtifactOptions{
		ValidateArtifact:  true,
		ArtifactIsBuiltIn: true})

	assert.NoError(self.T(), err)

	_, pres := repository.Get(self.Ctx, self.ConfigObj, "TestArtifact")
	assert.True(self.T(), pres)

	// The admin sets a very minimal tool definition which would
	// normally be less than the existing tool - but they should
	// prevail.
	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = inventory_service.AddTool(self.Ctx, self.ConfigObj,
		&artifacts_proto.Tool{
			Name: "SampleTool",
			Hash: "XXXXX",
		}, services.ToolOptions{AdminOverride: true})
	assert.NoError(self.T(), err)

	tool, err := inventory_service.ProbeToolInfo(
		self.Ctx, self.ConfigObj, "SampleTool", "")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), tool.Url, "")
	assert.Equal(self.T(), tool.Hash, "XXXXX")
	assert.False(self.T(), tool.ServeLocally)
}

// If the admin set the tool previously, they should be able to upgrade it.
func (self *ServicesTestSuite) TestAdminOverrideAdminSet() {
	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = inventory_service.AddTool(self.Ctx, self.ConfigObj,
		&artifacts_proto.Tool{
			Name: "SampleTool",
			Hash: "XXXXX",
		}, services.ToolOptions{AdminOverride: true})
	assert.NoError(self.T(), err)

	err = inventory_service.AddTool(self.Ctx, self.ConfigObj,
		&artifacts_proto.Tool{
			Name: "SampleTool",
			Hash: "YYYYY",
		}, services.ToolOptions{AdminOverride: true})
	assert.NoError(self.T(), err)

	tool, err := inventory_service.ProbeToolInfo(
		self.Ctx, self.ConfigObj, "SampleTool", "")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), tool.Url, "")
	assert.Equal(self.T(), tool.Hash, "YYYYY")
	assert.False(self.T(), tool.ServeLocally)
}

// Upload multiple versions - should co-exist.
func (self *ServicesTestSuite) TestMultipleVersions() {
	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Add the same tool with different versions
	err = inventory_service.AddTool(self.Ctx, self.ConfigObj,
		&artifacts_proto.Tool{
			Name:    "SampleTool",
			Version: "1",
			Hash:    "XXXXX",
		}, services.ToolOptions{AdminOverride: true})
	assert.NoError(self.T(), err)

	err = inventory_service.AddTool(self.Ctx, self.ConfigObj,
		&artifacts_proto.Tool{
			Name:    "SampleTool",
			Version: "2",
			Hash:    "YYYYY",
		}, services.ToolOptions{AdminOverride: true})
	assert.NoError(self.T(), err)

	tool, err := inventory_service.ProbeToolInfo(
		self.Ctx, self.ConfigObj, "SampleTool", "1")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), tool.Url, "")
	assert.Equal(self.T(), tool.Hash, "XXXXX")
	assert.False(self.T(), tool.ServeLocally)

	tool, err = inventory_service.ProbeToolInfo(
		self.Ctx, self.ConfigObj, "SampleTool", "2")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), tool.Url, "")
	assert.Equal(self.T(), tool.Hash, "YYYYY")
	assert.False(self.T(), tool.ServeLocally)
}

func (self *ServicesTestSuite) TestMultipleSemanticVersions() {
	// Define a new artifact that requires a new tool
	self.LoadArtifacts(`
name: TestArtifactSemver1
tools:
- name: SampleToolSemver
  url: http://www.example.com/SampleTool0.6.5.exe
  version: "0.6.5"
sources:
- query: SELECT Version065 FROM scope()
`, `
name: TestArtifactSemver2
tools:
- name: SampleToolSemver
  url: http://www.example.com/SampleTool0.6.5-rc2.exe
  version: "0.6.5-rc2"
sources:
- query: SELECT Version065_rc2 FROM scope()
`, `
name: TestArtifactSemverNoVersion
tools:
- name: SampleToolSemver
sources:
- query: SELECT HighestVersion FROM scope()
`, `
name: TestArtifactConflictingVersion
tools:
- name: SampleToolSemver
  url: http://www.example.com/ConflictingVersion.exe
  version: "0.6.5-rc2"
sources:
- query: SELECT Version065_rc2 FROM scope()
`)

	self.mock = &MockClient{
		responses: map[string]string{
			"http://www.example.com/SampleTool0.6.5.exe":     "File Content 1",
			"http://www.example.com/SampleTool0.6.5-rc2.exe": "File Content 2",
		},
	}

	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)
	inventory_service.(*inventory.InventoryService).Client = self.mock

	// Tools are only loaded when we compile the artifact since they are lazy.
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	response, err := launcher.CompileCollectorArgs(
		self.Ctx, self.ConfigObj, acl_managers.NullACLManager{}, repository,
		services.CompilerOptions{},
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{
				"TestArtifactSemver1",
				"TestArtifactSemver2",
				"TestArtifactSemverNoVersion",
				"TestArtifactConflictingVersion",
			},
		})
	assert.NoError(self.T(), err)

	// When an artifact defines a tool with a version they will
	// automatically use the version defined in that artifact.
	assert.Contains(self.T(), response[0].Query[0].VQL, "Version065")
	assert.Contains(self.T(), response[0].Env[1].Value, "SampleTool0.6.5.exe")

	assert.Contains(self.T(), response[1].Query[0].VQL, "Version065_rc2")
	assert.Contains(self.T(), response[1].Env[1].Value, "SampleTool0.6.5-rc2.exe")

	// If an artifact does not specify the version it will pick the
	// highest known version.
	assert.Contains(self.T(), response[2].Query[0].VQL, "HighestVersion")
	assert.Contains(self.T(), response[2].Env[1].Value, "SampleTool0.6.5.exe")

	// When no version is specified gets the latest version
	tool, err := inventory_service.ProbeToolInfo(
		self.Ctx, self.ConfigObj, "SampleToolSemver", "")
	assert.NoError(self.T(), err)

	// Make sure we get the latest version (0.6.5 is later than 0.6.5-rc2)
	assert.Equal(self.T(), tool.Version, "0.6.5")
	assert.Equal(self.T(), tool.Filename, "SampleTool0.6.5.exe")

	tool, err = inventory_service.GetToolInfo(
		self.Ctx, self.ConfigObj, "SampleToolSemver", "")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), tool.Version, "0.6.5")
	assert.Equal(self.T(), tool.Filename, "SampleTool0.6.5.exe")

	// But we can still explicitely get the old version if we wanted to
	tool, err = inventory_service.GetToolInfo(
		self.Ctx, self.ConfigObj, "SampleToolSemver", "0.6.5-rc2")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), tool.Version, "0.6.5-rc2")
	assert.Equal(self.T(), tool.Filename, "SampleTool0.6.5-rc2.exe")

	// 0.6.5-rc2 has two conflicting versions, the Tool description
	// should show both but not show any other version.
	assert.Equal(self.T(), 2, len(tool.Versions))

	// The active tool is defined in TestArtifactSemver2
	assert.Equal(self.T(), "TestArtifactSemver2", tool.Artifact)

	golden := ordereddict.NewDict().Set("Original Version", tool)

	// Now simulate the user overriding the active version in the GUI.
	err = inventory_service.AddTool(self.Ctx, self.ConfigObj,
		tool.Versions[1], services.ToolOptions{
			ArtifactDefinition: true,
		})
	assert.NoError(self.T(), err)

	tool, err = inventory_service.GetToolInfo(
		self.Ctx, self.ConfigObj, "SampleToolSemver", "0.6.5-rc2")
	assert.NoError(self.T(), err)

	// The active tool is defined in TestArtifactSemver2
	assert.Equal(self.T(), "TestArtifactConflictingVersion", tool.Artifact)

	golden.Set("Updated Version", tool)

	goldie.Assert(self.T(), "TestMultipleSemanticVersions",
		json.MustMarshalIndent(golden))
}

// Multiple artifacts with different versions
func (self *ServicesTestSuite) TestMultipleVersionsInArtifacts() {
	// Define a new artifact that requires a new tool
	self.LoadArtifacts(`
name: TestArtifact1
tools:
- name: SampleTool
  url: http://www.example.com/SampleTool1.exe
  version: "1"
`, `
name: TestArtifact2
tools:
- name: SampleTool
  url: http://www.example.com/SampleTool2.exe
  version: "2"
`)

	self.mock = &MockClient{
		responses: map[string]string{
			"http://www.example.com/SampleTool2.exe": "File Content 2",
			"http://www.example.com/SampleTool1.exe": "File Content 1",
		},
	}

	inventory_service, err := services.GetInventory(self.ConfigObj)
	assert.NoError(self.T(), err)
	inventory_service.(*inventory.InventoryService).Client = self.mock

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	response, err := launcher.CompileCollectorArgs(
		self.Ctx, self.ConfigObj, acl_managers.NullACLManager{}, repository,
		services.CompilerOptions{},
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact1"},
		})
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), response[0].Env[0].Value,
		getHash("File Content 1"))
	assert.Equal(self.T(), response[0].Env[2].Value,
		"http://www.example.com/SampleTool1.exe")

	response, err = launcher.CompileCollectorArgs(
		self.Ctx, self.ConfigObj, acl_managers.NullACLManager{}, repository,
		services.CompilerOptions{},
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact2"},
		})
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), response[0].Env[0].Value,
		getHash("File Content 2"))
	assert.Equal(self.T(), response[0].Env[2].Value,
		"http://www.example.com/SampleTool2.exe")
}

func getHash(data string) string {
	sha_sum := sha256.New()
	sha_sum.Write([]byte(data))
	return hex.EncodeToString(sha_sum.Sum(nil))
}

func TestInventoryService(t *testing.T) {
	suite.Run(t, &ServicesTestSuite{
		client_id: "C.12312",
		flow_id:   "F.1232",
	})
}
