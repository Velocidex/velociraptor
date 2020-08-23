package inventory

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type MockClient struct {
	responses map[string]string

	count int
}

func (self MockClient) Do(req *http.Request) (*http.Response, error) {
	url := req.URL.String()

	response := self.responses[url]
	delete(self.responses, url)

	self.count++

	return &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewReader([]byte(response))),
	}, nil
}

type ServicesTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	client_id  string
	flow_id    string
	sm         *services.Service
	mock       *MockClient
}

func (self *ServicesTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
	require.NoError(self.T(), self.sm.Start(StartInventoryService))

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
}

func (self *ServicesTestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

func (self *ServicesTestSuite) TestGihubTools() {
	ctx := context.Background()
	tool_name := "SampleTool"
	golden := ordereddict.NewDict()

	self.installGitHubMock()

	// Add a new tool from github.
	inventory := services.GetInventory()
	err := inventory.AddTool(
		self.config_obj, &artifacts_proto.Tool{
			Name:             tool_name,
			GithubProject:    "Velocidex/velociraptor",
			GithubAssetRegex: "windows-amd64.exe",
		})
	assert.NoError(self.T(), err)

	// Adding the tool simply fetches the github url but not the
	// actual file (the URL is still pending).
	assert.Equal(self.T(), self.mock.count, 0)

	tool, err := inventory.GetToolInfo(ctx, self.config_obj, tool_name)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), len(self.mock.responses), 0)

	// Both HTTP requests were made - Getting the tool info
	// downloads the file from the server.
	assert.Equal(self.T(), len(self.mock.responses), 0)

	assert.Equal(self.T(), tool.Name, "SampleTool")
	assert.Equal(self.T(), tool.Url, "htttp://www.example.com/file.exe")

	golden.Set("Tool", tool)

	// What does the launcher do?
	request := &actions_proto.VQLCollectorArgs{}
	err = launcher.AddToolDependency(ctx, self.config_obj, tool_name, request)
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

	inventory := services.GetInventory().(*InventoryService)
	inventory.Client = self.mock
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

	inventory := services.GetInventory().(*InventoryService)
	inventory.Client = self.mock
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
	repository := services.GetRepositoryManager().NewRepository()
	_, err := repository.LoadYaml(test_artifact, true /* validate */)
	assert.NoError(self.T(), err)

	self.installGitHubMock()

	// Launch the artifact - this will result in the tool being
	// downloaded and the hash calculated on demand.
	response, err := services.GetLauncher().CompileCollectorArgs(
		ctx, self.config_obj, vql_subsystem.NullACLManager{}, repository,
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
		})
	assert.NoError(self.T(), err)

	// What is the tool info - should have resolved the final
	// destination and the hash.
	tool_name := "SampleTool"
	inventory := services.GetInventory()
	tool, err := inventory.GetToolInfo(ctx, self.config_obj, tool_name)
	assert.NoError(self.T(), err)

	// Make sure the tool is served directly from upstream.
	assert.Equal(self.T(), response.Env[2].Key, "Tool_SampleTool_URL")
	assert.Equal(self.T(), response.Env[2].Value, "htttp://www.example.com/file.exe")

	assert.Equal(self.T(), response.Env[0].Key, "Tool_SampleTool_HASH")
	assert.Equal(self.T(), response.Env[0].Value,
		"3c03cf5341a1e078c438f31852e1587a70cc9f91ee02eda315dd231aba0a0ab1")

	golden := ordereddict.NewDict().Set("Tool", tool).Set("Request", response)
	serialized, err := json.MarshalIndentNormalized(golden)
	assert.NoError(self.T(), err)
	goldie.Assert(self.T(), "TestGihubToolsUninitialized", serialized)
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

	inventory := services.GetInventory()
	err := inventory.AddTool(self.config_obj, tool_definition)
	assert.NoError(self.T(), err)

	tool, err := inventory.GetToolInfo(ctx, self.config_obj, tool_name)
	assert.NoError(self.T(), err)

	// First version.
	assert.Equal(self.T(), tool.Url, "htttp://www.example.com/file.exe")
	assert.Equal(self.T(), tool.Hash, "3c03cf5341a1e078c438f31852e1587a70cc9f91ee02eda315dd231aba0a0ab1")

	// Now force the tool to update by re-adding it but this time it is a new version.
	self.installGitHubMockVersion2()

	err = inventory.AddTool(self.config_obj, tool_definition)
	assert.NoError(self.T(), err)

	// Check the tool information.
	tool, err = inventory.GetToolInfo(ctx, self.config_obj, tool_name)
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
	repository := services.GetRepositoryManager().NewRepository()
	_, err := repository.LoadYaml(test_artifact, true /* validate */)
	assert.NoError(self.T(), err)

	self.installGitHubMock()

	response, err := services.GetLauncher().CompileCollectorArgs(
		ctx, self.config_obj, vql_subsystem.NullACLManager{}, repository,
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
		})
	assert.NoError(self.T(), err)

	// Make sure the tool is served directly from the public directory.
	assert.Equal(self.T(), response.Env[2].Key, "Tool_SampleTool_URL")
	assert.Contains(self.T(), response.Env[2].Value, "https://localhost:8000/")

	tool, err := services.GetInventory().GetToolInfo(
		ctx, self.config_obj, "SampleTool")
	assert.NoError(self.T(), err)

	golden := ordereddict.NewDict().Set("Tool", tool).Set("Request", response)
	serialized, err := json.MarshalIndentNormalized(golden)
	assert.NoError(self.T(), err)
	goldie.Assert(self.T(), "TestGihubToolServedLocally", serialized)
}

func TestInventoryService(t *testing.T) {
	suite.Run(t, &ServicesTestSuite{})
}
