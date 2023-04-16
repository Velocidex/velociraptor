package downloads

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/server/clients"
	"www.velocidex.com/golang/velociraptor/vql/server/hunts"
	"www.velocidex.com/golang/velociraptor/vql/tools/collector"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/vql/protocols"
)

type TestSuite struct {
	test_utils.TestSuite
	client_id string
}

func (self *TestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.HuntDispatcher = true
	self.ConfigObj.Services.HuntManager = true

	self.LoadArtifactsIntoConfig([]string{`
name: Custom.TestArtifactUpload
type: CLIENT
sources:
- query: SELECT * FROM info()
`})

	self.TestSuite.SetupTest()

	Clock = &utils.MockClock{MockNow: time.Unix(1602103388, 0)}
	reporting.Clock = Clock
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)
	launcher.SetFlowIdForTests("F.1234")
}

func (self *TestSuite) TestExportCollectionServerArtifact() {
	import_file_path, err := filepath.Abs("fixtures/export_server_artifact.zip")
	assert.NoError(self.T(), err)

	test_utils.UnzipToFilestore(self.ConfigObj,
		path_specs.NewUnsafeFilestorePath("clients", "server", "collections"),
		import_file_path)

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()
	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := self.Ctx
	scope := manager.BuildScope(builder)

	// Now create the download export. The plugin returns a filestore
	// pathspec to the created download file.
	result := (&CreateFlowDownload{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", "server").
			Set("flow_id", "F.CGLR6OS84DP00").
			Set("wait", true).
			Set("expand_sparse", false).
			Set("name", "Test"))

	// A zip file was created
	path_spec, ok := result.(path_specs.FSPathSpec)
	assert.True(self.T(), ok)

	file_details, err := openZipFile(self.ConfigObj, scope, path_spec)
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestExportCollectionServerArtifact",
		json.MustMarshalIndent(file_details))
}

// First import a collection from a zip file to create a
// collection. Then we export the collection back into zip files to
// test the export functionality.
func (self *TestSuite) TestExportCollection() {
	manager, _ := services.GetRepositoryManager(self.ConfigObj)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := self.Ctx
	scope := manager.BuildScope(builder)

	import_file_path, err := filepath.Abs("fixtures/export.zip")
	assert.NoError(self.T(), err)

	result := collector.ImportCollectionFunction{}.Call(ctx, scope,
		ordereddict.NewDict().
			// Set a fixed client id to keep it predictable
			Set("client_id", self.client_id).
			Set("hostname", "MyNewHost").
			Set("filename", import_file_path))
	context, ok := result.(*flows_proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)
	assert.Equal(self.T(), uint64(11), context.TotalUploadedBytes)

	// Now create the download export. The plugin returns a filestore
	// pathspec to the created download file.
	result = (&CreateFlowDownload{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", context.ClientId).
			Set("flow_id", context.SessionId).
			Set("wait", true).
			Set("expand_sparse", false).
			Set("name", "Test"))

	// A zip file was created
	path_spec, ok := result.(path_specs.FSPathSpec)
	assert.True(self.T(), ok)

	assert.Equal(self.T(),
		"fs:/downloads/"+self.client_id+"/F.1234/Test.zip", path_spec.String())

	// Now inspect the zip file
	file_details, err := openZipFile(self.ConfigObj, scope, path_spec)
	assert.NoError(self.T(), err)

	// Ensure the zip file contains the sparse file and index as well
	// as unsparse.
	file_content, _ := file_details.GetString(
		"uploads/data/file.txt")
	assert.Equal(self.T(), "hello world", file_content)

	file_content, _ = file_details.GetString(
		"uploads/sparse/C%3A/file.sparse.txt")
	assert.Equal(self.T(), "This bit", file_content)

	// Make sure we have an index file
	_, pres := file_details.Get("uploads/sparse/C%3A/file.sparse.txt.idx")
	assert.True(self.T(), pres)

	// Now create the download export with sparse files expanded.
	result = (&CreateFlowDownload{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", context.ClientId).
			Set("flow_id", context.SessionId).
			Set("wait", true).
			Set("expand_sparse", true).
			Set("name", "TestExpanded"))

	// A zip file was created
	path_spec, ok = result.(path_specs.FSPathSpec)
	assert.True(self.T(), ok)

	assert.Equal(self.T(),
		"fs:/downloads/"+self.client_id+"/F.1234/TestExpanded.zip", path_spec.String())

	// Now inspect the zip file
	file_details, err = openZipFile(self.ConfigObj, scope, path_spec)
	assert.NoError(self.T(), err)

	file_content, _ = file_details.GetString(
		"uploads/sparse/C%3A/file.sparse.txt")
	// File should have a zero padded region between 5 and 10 bytes
	assert.Equal(self.T(), "This \x00\x00\x00\x00\x00bit", file_content)

	// No idx file is emitted because we expanded all files.
	_, pres = file_details.Get("uploads/sparse/C%3A/file.sparse.txt.idx")
	assert.True(self.T(), !pres)

	uploads_json, pres := file_details.Get("uploads.json")
	assert.True(self.T(), pres)

	goldie.Assert(self.T(), "TestExportCollectionUploads",
		json.MustMarshalIndent(uploads_json))
}

func (self *TestSuite) TestExportHunt() {
	// Operate on a different client
	self.client_id = "C.1235"

	manager, _ := services.GetRepositoryManager(self.ConfigObj)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := context.Background()
	scope := manager.BuildScope(builder)

	import_file_path, err := filepath.Abs("fixtures/export.zip")
	assert.NoError(self.T(), err)

	// Create a new client
	result := (&clients.NewClientFunction{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", self.client_id).
			Set("hostname", "TestClient"))

	client_info := result.(actions_proto.ClientInfo)
	assert.Equal(self.T(), self.client_id, client_info.ClientId)

	result = collector.ImportCollectionFunction{}.Call(ctx, scope,
		ordereddict.NewDict().
			// Set a fixed client id to keep it predictable
			Set("client_id", self.client_id).
			Set("hostname", "MyNewHost").
			Set("filename", import_file_path))
	context, ok := result.(*flows_proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)
	assert.Equal(self.T(), uint64(11), context.TotalUploadedBytes)

	flow_id := context.SessionId

	hunt_dispatcher.SetHuntIdForTests("H.123")

	// Create a hunt and add the flow to it.
	result = (&hunts.ScheduleHuntFunction{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("artifacts", "Custom.TestArtifactUpload").
			Set("pause", true))

	hunt_id, pres := result.(*ordereddict.Dict).GetString("HuntId")
	assert.True(self.T(), pres && hunt_id != "")

	// Now add the collection to the hunt.
	result = (&hunts.AddToHuntFunction{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", self.client_id).
			Set("hunt_id", hunt_id).
			Set("flow_id", flow_id))

	assert.Equal(self.T(), self.client_id, result.(string))

	time.Sleep(time.Second)

	// Now create a hunt download export.
	result = (&CreateHuntDownload{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("hunt_id", hunt_id).
			Set("base", "HuntExport").
			// Test the CSV production
			Set("format", "csv").
			Set("wait", true))

	download_pathspec := result.(path_specs.FSPathSpec)
	assert.Equal(self.T(), "/downloads/hunts/H.123/HuntExportH.123.zip",
		download_pathspec.AsClientPath())

	// Now inspect the zip file
	file_details, err := openZipFile(self.ConfigObj, scope, download_pathspec)
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestExportHunt",
		json.MustMarshalIndent(file_details))
}

func TestDownloadsPlugin(t *testing.T) {
	suite.Run(t, &TestSuite{
		client_id: "C.1234",
	})
}

// Read the entire zip file for inspection.
func openZipFile(
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	src api.FSPathSpec) (*ordereddict.Dict, error) {
	file_store_factory := file_store.GetFileStore(config_obj)

	// When we exit from here we make sure to remove this file to
	// cleanup
	defer file_store_factory.Delete(src)

	reader, err := file_store_factory.ReadFile(src)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	file_info, err := reader.Stat()
	if err != nil {
		return nil, err
	}

	result := ordereddict.NewDict()

	r, err := zip.NewReader(utils.MakeReaderAtter(reader), file_info.Size())
	if err != nil {
		return nil, err
	}

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
