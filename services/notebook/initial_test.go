package notebook_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"
)

const DEBUG = false

var InitialArtifacts = []string{`
name: Generic.Client.Info
type: CLIENT
parameters:
- name: FirstParameter
  type: timestamp
  default: "1982-12-10"

sources:
- query: |
    SELECT * FROM info()
`, `
name: Custom.Generic.Client.Info
type: CLIENT
sources:
- query: |
    SELECT * FROM info()
  notebook:
  - type: markdown
    template: |
      # Custom.Generic.Client.Info template
`, `
name: Generic.Events
type: CLIENT_EVENT
parameters:
- name: Arg1
  default: Foo

sources:
- query: |
    SELECT * FROM info()
`, `
name: GlobalNotebook
parameters:
- name: Arg1
  default: Foo
- name: Arg2
  default: Bar

sources:
- notebook:
  - type: vql
    template: |
       SELECT Arg1 FROM scope()
`, `
name: ArtifactWithExport
type: CLIENT
export: |
   LET ArtifactExport = 1
`, `
name: NotebookWithImport
type: NOTEBOOK
imports:
- ArtifactWithExport
export: |
  LET NotebookExport = 2
sources:
- notebook:
   - type: vql
     template: SELECT NotebookExport, ArtifactExport FROM scope()
`, `
name: NotebookWithSugegstions
type: NOTEBOOK
sources:
- query: SELECT * FROM info()
  notebook:
  - name: A good suggestion.
    type: vql_suggestion
    template: SELECT * FROM info()
`}

type checker func(t *testing.T, response *artifacts_proto.Artifact, spec *flows_proto.ArtifactSpec)

type notebookTestCase struct {
	req   *api_proto.NotebookMetadata
	check checker
}

func initialTestCases(client_id string) []notebookTestCase {
	return []notebookTestCase{
		{
			req: &api_proto.NotebookMetadata{
				Name:        "Unspecified Artifacts",
				Description: "Should return the default notebook artifact",
			},
			check: func(t *testing.T, response *artifacts_proto.Artifact,
				spec *flows_proto.ArtifactSpec) {
				AssertDictRegex(t,
					"Welcome to Velociraptor",
					"Sources.0.Notebook.0.Template", response)
			},
		},

		{
			req: &api_proto.NotebookMetadata{
				Name: "Client artifact",

				// Client notebooks have predetermined notebook id. This
				// should cause the PrivateNotebook to add client id and
				// flow id to the artifact parameters.
				NotebookId:  "N.F.1234-" + client_id,
				Description: "Notebook based on a client artifact with no custom notebooks.",
				Artifacts:   []string{"Generic.Client.Info"},
			},
			check: func(t *testing.T, artifact *artifacts_proto.Artifact,
				spec *flows_proto.ArtifactSpec) {

				// The artifact defaults are maintained.
				AssertDictRegex(t, "FirstParameter", "Parameters.0.Name", artifact)
				AssertDictRegex(t, "1982-12-10", "Parameters.0.Default", artifact)

				// ClientId and Flow ID are added
				AssertDictRegex(t, "ClientId", "Parameters.2.Name", artifact)
				AssertDictRegex(t, client_id, "Parameters.2.Default", artifact)

				AssertDictRegex(t, "FlowId", "Parameters.3.Name", artifact)
				AssertDictRegex(t, "F.1234", "Parameters.3.Default", artifact)

				// But the spec contains the actual collected data
				AssertDictRegex(t, "FirstParameter", "Parameters.Env.0.Key", spec)
				AssertDictRegex(t, "2022-11", "Parameters.Env.0.Value", spec)
			},
		},
		{
			req: &api_proto.NotebookMetadata{
				Name:        "Custom.Generic.Client.Info",
				NotebookId:  "N.F.1235-" + client_id,
				Description: "Based on custom notebook cells",
				Artifacts:   []string{"Custom.Generic.Client.Info"},
			},
			check: func(t *testing.T, artifact *artifacts_proto.Artifact,
				spec *flows_proto.ArtifactSpec) {
				AssertDictRegex(t, "Custom.Generic.Client.Info template",
					"Sources.0.Notebook.0.Template", artifact)
			},
		},
		{
			req: &api_proto.NotebookMetadata{
				Name:        "EventArtifact",
				Description: "Building a notebook from an event artifact adds StartTime and EndTime",
				NotebookId:  "N.E.Generic.Events-" + client_id,
				Artifacts:   []string{"Generic.Events"},
				Env: []*api_proto.Env{{
					Key: "StartTime", Value: "2020-01-10",
				}},
			},
			check: func(t *testing.T, artifact *artifacts_proto.Artifact,
				spec *flows_proto.ArtifactSpec) {

				// StartTime and EndTime are added as parameters
				AssertDictRegex(t, "StartTime", "Parameters.3.Name", artifact)
				AssertDictRegex(t, "EndTime", "Parameters.4.Name", artifact)

				// The Value of StartTime in the spec comes from the
				// Env of the request.
				AssertDictRegex(t, "StartTime", "Parameters.Env.1.Key", spec)
				AssertDictRegex(t, "2020-01", "Parameters.Env.1.Value", spec)

				AssertDictRegex(t, "Arg1", "Parameters.Env.0.Key", spec)
				AssertDictRegex(t, "Custom Arg1", "Parameters.Env.0.Value", spec)

			},
		},
		{
			req: &api_proto.NotebookMetadata{
				Name:        "EventArtifact Default",
				Description: "Building a notebook from an event artifact without custom notebooks. This should populate the spec from the installed client event monitoring table.",
				NotebookId:  "N.E.Generic.Events-" + client_id,
				Artifacts:   []string{"Generic.Events"},
				Env: []*api_proto.Env{
					{Key: "StartTime", Value: "2024-10"},
				},
			},
			check: func(t *testing.T, artifact *artifacts_proto.Artifact,
				spec *flows_proto.ArtifactSpec) {
				AssertDictRegex(t, "Arg1", "Parameters.Env.0.Key", spec)
				AssertDictRegex(t, "Custom Arg1", "Parameters.Env.0.Value", spec)

				AssertDictRegex(t, "StartTime",
					"Parameters.Env.1.Key", spec)
				AssertDictRegex(t, "2024-10", "Parameters.Env.1.Value", spec)
			},
		},
		{
			req: &api_proto.NotebookMetadata{
				Name:        "Create GlobalNotebook",
				Description: "Creating a global notebook with specs",
				// No notebook id provided - generate a random one.
				Artifacts: []string{"GlobalNotebook"},
				Specs: []*flows_proto.ArtifactSpec{{
					Artifact: "GlobalNotebook",
					Parameters: &flows_proto.ArtifactParameters{
						Env: []*actions_proto.VQLEnv{
							{Key: "Arg1", Value: "Hello"},
							{Key: "Arg2", Value: "World"},
						},
					},
				}},
			},
			check: func(t *testing.T, artifact *artifacts_proto.Artifact,
				spec *flows_proto.ArtifactSpec) {
				// Include the artifact default parameters
				AssertDictRegex(t, "Arg1", "Parameters.0.Name", artifact)
				AssertDictRegex(t, "Foo", "Parameters.0.Default", artifact)

				// But the spec reflects what the request put in the env.
				AssertDictRegex(t, "Hello", "Parameters.Env.0.Value", spec)
			},
		},
		{
			req: &api_proto.NotebookMetadata{
				Name:        "Create Hunt Notebook",
				Description: "Creating a notebook from hunt view",
				// No notebook id provided - generate a random one.
				Artifacts:  []string{"Generic.Client.Info"},
				NotebookId: "N.H.1234",
			},
			check: func(t *testing.T, artifact *artifacts_proto.Artifact,
				spec *flows_proto.ArtifactSpec) {

				// HuntId is added
				AssertDictRegex(t, "HuntId", "Parameters.2.Name", artifact)
				AssertDictRegex(t, "H.1234", "Parameters.2.Default", artifact)

				// Spec has the value from the hunt object
				AssertDictRegex(t, "FirstParameter", "Parameters.Env.0.Key", spec)
				AssertDictRegex(t, "2021-11", "Parameters.Env.0.Value", spec)

			},
		},
		{
			req: &api_proto.NotebookMetadata{
				Name:      "Notebook with exports",
				Artifacts: []string{"NotebookWithImport"},
			},
			check: func(t *testing.T, artifact *artifacts_proto.Artifact,
				spec *flows_proto.ArtifactSpec) {

				// Both exports should be visible.
				AssertDictRegex(t, "NotebookExport", "Export", artifact)
				AssertDictRegex(t, "ArtifactExport", "Export", artifact)
			},
		},
		{
			req: &api_proto.NotebookMetadata{
				Name:        "Notebook with suggestions",
				Description: "Adding a custom notebook with suggestion will add the default notebook",
				Artifacts:   []string{"NotebookWithSugegstions"},
			},
			check: func(t *testing.T, artifact *artifacts_proto.Artifact,
				spec *flows_proto.ArtifactSpec) {

				AssertDictRegex(t, "A good suggestion", "Sources.0.Notebook.0.Name", artifact)
				AssertDictRegex(t, "vql_suggestion", "Sources.0.Notebook.0.Type", artifact)
				AssertDictRegex(t, "SELECT \\* FROM source", "Sources.0.Notebook.1.Template", artifact)
				AssertDictRegex(t, "vql", "Sources.0.Notebook.1.Type", artifact)
			},
		},
	}
}

func (self *NotebookManagerTestSuite) createFlow(
	flow_id, client_id, artifact string) {

	acl_manager := acl_managers.NewServerACLManager(self.ConfigObj, "admin")

	// Create a hunt
	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(self.T(), err)
	_, err = hunt_dispatcher.CreateHunt(self.Ctx, self.ConfigObj,
		acl_manager, &api_proto.Hunt{
			HuntId: "H.1234",
			State:  api_proto.Hunt_RUNNING,
			StartRequest: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"Generic.Client.Info"},
				Specs: []*flows_proto.ArtifactSpec{{
					Artifact: "Generic.Client.Info",
					// Override the FirstParameter in this hunt
					Parameters: &flows_proto.ArtifactParameters{
						Env: []*actions_proto.VQLEnv{
							// Override the default when scheduling the artifact.
							{Key: "FirstParameter", Value: "2021-11-10"},
						},
					},
				}},
			},
		})
	assert.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)

	defer utils.SetFlowIdForTests(flow_id)()

	_, err = launcher.ScheduleArtifactCollection(self.Ctx, self.ConfigObj, acl_manager,
		repository, &flows_proto.ArtifactCollectorArgs{
			Creator:   "admin",
			ClientId:  client_id,
			Artifacts: []string{artifact},
			Specs: []*flows_proto.ArtifactSpec{{
				Artifact: artifact,
				Parameters: &flows_proto.ArtifactParameters{
					Env: []*actions_proto.VQLEnv{
						// Override the default when scheduling the artifact.
						{Key: "FirstParameter", Value: "2022-11-10"},
					},
				},
			}},
		}, nil)
	assert.NoError(self.T(), err)

	// Start some client monitoring
	client_monitoring_service, err := services.ClientEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = client_monitoring_service.SetClientMonitoringState(
		self.Ctx, self.ConfigObj, "admin", &flows_proto.ClientEventTable{
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"Generic.Events"},
				Specs: []*flows_proto.ArtifactSpec{{
					Artifact: "Generic.Events",
					Parameters: &flows_proto.ArtifactParameters{
						Env: []*actions_proto.VQLEnv{
							{Key: "Arg1", Value: "Custom Arg1"},
						},
					},
				}},
			},
		})
	assert.NoError(self.T(), err)
}

func (self *NotebookManagerTestSuite) TestInitialNotebook() {
	self.LoadArtifacts(InitialArtifacts...)

	self.createFlow("F.1234", self.client_id, "Generic.Client.Info")
	self.createFlow("F.1235", self.client_id, "Custom.Generic.Client.Info")

	golden := ordereddict.NewDict()
	for _, tc := range initialTestCases(self.client_id) {
		req := tc.req
		golden.Set(req.Name+" Request", proto.Clone(req))

		if req.Name == "Create GlobalNotebook" {
			utils.DlvBreak()
		}

		artifact, out, err := notebook.CalculateNotebookArtifact(
			self.Ctx, self.ConfigObj, req)
		assert.NoError(self.T(), err)

		golden.Set(req.Name+" Response", proto.Clone(artifact))

		spec, err := notebook.CalculateSpecs(
			self.Ctx, self.ConfigObj, artifact, out)
		assert.NoError(self.T(), err)

		golden.Set(req.Name+" Spec", proto.Clone(spec))

		if DEBUG {
			fmt.Printf("Checking %v\n", tc.req)
		}
		if tc.check != nil {
			tc.check(self.T(), artifact, spec)
		}
	}
	goldie.Assert(self.T(), "TestInitialNotebook",
		json.MustMarshalIndent(golden))

}

func AssertDictRegex(t *testing.T, regex, selector string, item protoreflect.ProtoMessage) {
	field := GetField(selector, item)
	assert.Regexp(t, regex, field, "%v", item)
}

func GetField(selector string, item protoreflect.ProtoMessage) string {
	scope := vql_subsystem.MakeScope()
	ctx := context.Background()
	var result interface{} = vfilter.RowToDict(
		ctx, scope, proto.Clone(item)).SetCaseInsensitive()
	var pres bool

	for _, member := range strings.Split(selector, ".") {
		int_member, err := strconv.Atoi(member)
		if err == nil {
			result, pres = scope.Associative(result, int_member)
		} else {
			result, pres = scope.Associative(result, member)
		}

		if DEBUG {
			fmt.Printf("Member for %v is %v\n", member, result)
		}

		if !pres {
			return ""
		}
	}

	return fmt.Sprintf("%v", result)
}
