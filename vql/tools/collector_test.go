package tools

import (
	"archive/zip"
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/indexing"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"

	// Load all needed plugins
	_ "www.velocidex.com/golang/velociraptor/accessors/data"
	_ "www.velocidex.com/golang/velociraptor/vql/functions"
	_ "www.velocidex.com/golang/velociraptor/vql/networking"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers/csv"
)

var (
	simpleCollectorArgs = &CollectPluginArgs{
		Artifacts: []string{"Demo.Plugins.GUI"},
		Args: ordereddict.NewDict().
			Set("Demo.Plugins.GUI", ordereddict.NewDict().

				// Bools will be converted to a "Y"
				Set("OffFlag", true).
				Set("ChoiceSelector", "InvalidChoice").
				Set("Flag", "N"). // Setting to "N" is the same as false.
				Set("Flag2", false).

				// Time object
				Set("StartDate", time.Unix(1608015035, 0)).

				// Int
				Set("StartDate2", 1608015035).

				// Float
				Set("StartDate3", 1608015035.0).

				// Stuffing rows data into a CSV field
				// should convert to csv.
				Set("CSVData", []*ordereddict.Dict{
					ordereddict.NewDict().
						Set("Foo", "Bar").
						Set("Baz", "Baz"),
					ordereddict.NewDict().
						Set("Foo", "Bar2").
						Set("Baz", "Baz2"),
				}).

				// Stuffing arbitrary data into a json
				// field should convert to json.
				Set("JSONData", []*ordereddict.Dict{
					ordereddict.NewDict().
						Set("Foo", "Bar").
						Set("Baz", "Baz"),
					ordereddict.NewDict().
						Set("Foo", "Bar2").
						Set("Baz", "Baz2"),
				}).

				// Add some spurious args, they are
				// not valid and will warn but be included.
				Set("InvalidArg", "InvalidArgValue")),
	}

	// Test case that adds a new artifact definition then calls it
	// with a new parameter.
	additionalArtifactCollectorArgs = ordereddict.NewDict().
					Set("artifacts", []string{"Custom.TestArtifactDependent"}).
					Set("args", ordereddict.NewDict().
						Set("Custom.TestArtifactDependent", ordereddict.NewDict().
				// This will be injected into the output zip by the below artifact.
				Set("FooVar", "HelloFooVar"))).
		Set("artifact_definitions", CustomTestArtifactDependent)

	CustomTestArtifactDependent = `
name: Custom.TestArtifactDependent
parameters:
- name: FooVar
sources:
- query: SELECT FooVar FROM scope()

reports:
- type: CLIENT
  template: |
     This is a template.
     {{ Query "SELECT * FROM source()" | Table }}

`
	uploadArtifactCollectorArgs = ordereddict.NewDict().
					Set("artifacts", []string{"Custom.TestArtifactUpload"}).
					Set("artifact_definitions", `
name: Custom.TestArtifactUpload
sources:
- query: |
    SELECT upload(file="hello world", accessor="data", name="file.txt") AS Upload FROM scope()
`)
)

type TestSuite struct {
	test_utils.TestSuite
}

func (self *TestSuite) SetupTest() {
	self.TestSuite.SetupTest()
	self.LoadArtifactFiles(
		"../../artifacts/definitions/Demo/Plugins/GUI.yaml",
		"../../artifacts/definitions/Reporting/Default.yaml",
	)

	require.NoError(self.T(), self.Sm.Start(indexing.StartIndexingService))
}

func (self *TestSuite) TestSimpleCollection() {
	scope := vql_subsystem.MakeScope()

	scope.SetLogger(logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent))

	repository, err := getRepository(scope, self.ConfigObj, nil)
	assert.NoError(self.T(), err)

	request, err := getArtifactCollectorArgs(self.ConfigObj,
		repository, scope, simpleCollectorArgs)
	assert.NoError(self.T(), err)

	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	acl_manager := vql_subsystem.NullACLManager{}
	vql_requests, err := launcher.CompileCollectorArgs(
		context.Background(), self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)

	serialized, err := json.MarshalIndent(ordereddict.NewDict().
		Set("ArtifactCollectorArgs", request).
		Set("vql_requests", vql_requests))
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestSimpleCollection", serialized)
}

func (self *TestSuite) TestCollectionWithArtifacts() {
	output_file, err := ioutil.TempFile(os.TempDir(), "zip")
	assert.NoError(self.T(), err)
	output_file.Close()

	defer os.Remove(output_file.Name())

	report_file, err := ioutil.TempFile(os.TempDir(), "html")
	assert.NoError(self.T(), err)
	report_file.Close()
	defer os.Remove(report_file.Name())

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	additionalArtifactCollectorArgs.Set("output", output_file.Name())
	additionalArtifactCollectorArgs.Set("report", report_file.Name())

	results := []vfilter.Row{}
	for row := range (CollectPlugin{}).Call(context.Background(),
		scope, additionalArtifactCollectorArgs) {
		results = append(results, row)
	}

	zip_contents, err := openZipFile(output_file.Name())
	assert.NoError(self.T(), err)

	fd, err := os.Open(report_file.Name())
	assert.NoError(self.T(), err)
	report_data, err := ioutil.ReadAll(fd)
	assert.NoError(self.T(), err)

	// Ensure the variable ends up inside the report.
	assert.Contains(self.T(), string(report_data), "HelloFooVar")

	serialized, err := json.MarshalIndent(ordereddict.NewDict().
		Set("zip_contents", zip_contents))
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestCollectionWithArtifacts", serialized)
}

func (self *TestSuite) TestCollectionWithTypes() {
	output_file, err := ioutil.TempFile(os.TempDir(), "zip")
	assert.NoError(self.T(), err)
	output_file.Close()
	defer os.Remove(output_file.Name())

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	results := []vfilter.Row{}
	args := ordereddict.NewDict().
		Set("artifacts", []string{"Demo.Plugins.GUI"}).
		Set("output", output_file.Name()).
		Set("args", simpleCollectorArgs.Args)

	for row := range (CollectPlugin{}).Call(context.Background(),
		scope, args) {
		results = append(results, row)
	}

	zip_contents, err := openZipFile(output_file.Name())
	assert.NoError(self.T(), err)

	serialized, err := json.MarshalIndent(ordereddict.NewDict().
		Set("zip_contents", zip_contents))
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestCollectionWithTypes", serialized)
}

func (self *TestSuite) TestCollectionWithUpload() {
	output_file, err := ioutil.TempFile(os.TempDir(), "zip")
	assert.NoError(self.T(), err)
	output_file.Close()
	defer os.Remove(output_file.Name())

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	results := []vfilter.Row{}

	// Set the output file.
	uploadArtifactCollectorArgs.Set("output", output_file.Name())

	for row := range (CollectPlugin{}).Call(context.Background(),
		scope, uploadArtifactCollectorArgs) {
		results = append(results, row)
	}

	zip_contents, err := openZipFile(output_file.Name())
	assert.NoError(self.T(), err)

	serialized := json.MustMarshalIndent(ordereddict.NewDict().
		Set("zip_contents", zip_contents))
	goldie.Assert(self.T(), "TestCollectionWithUpload", serialized)
}

func openZipFile(name string) (*ordereddict.Dict, error) {
	result := ordereddict.NewDict()

	r, err := zip.OpenReader(name)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		serialized, err := ioutil.ReadAll(rc)
		if err != nil {
			return nil, err
		}

		rows, err := utils.ParseJsonToDicts(serialized)
		if err != nil {
			result.Set(f.Name, string(serialized))
			continue
		}

		result.Set(f.Name, rows)
	}

	return result, nil
}

func TestCollectorPlugin(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
