package collector_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/flows/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/server/downloads"
	"www.velocidex.com/golang/velociraptor/vql/server/flows"
	"www.velocidex.com/golang/velociraptor/vql/tools/collector"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	file_store_accessor "www.velocidex.com/golang/velociraptor/accessors/file_store"
	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
	_ "www.velocidex.com/golang/velociraptor/vql/protocols"
)

const (
	TestFrontendCertificate = `-----BEGIN CERTIFICATE-----
MIIDWTCCAkGgAwIBAgIQcyUFy1oMUr4O4sIOhom/jDANBgkqhkiG9w0BAQsFADAa
MRgwFgYDVQQKEw9WZWxvY2lyYXB0b3IgQ0EwIBcNMjMwNDEzMTgzMjUzWhgPMjEy
MzAzMjAxODMyNTNaMDQxFTATBgNVBAoTDFZlbG9jaXJhcHRvcjEbMBkGA1UEAxMS
VmVsb2NpcmFwdG9yU2VydmVyMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
AQEA9MSMbrFjmZs9bnpkel4vTQIyf+6Bpg60ByC7d6WWfBwvHdF1Qnfn1JO3Xo6p
53I1jPoagt0cZCzd6nwJXJ/3pclprmIOEBSc20pg5E0A/kpwn+bBoPNSrMF7+2/t
DvXP0Lvs/1OqUMjF8pCs6vnSKigaptn+0Et3GpzWjwCghqPcJBOuEuPQmR3HyHfs
dsMooCjuYcRcS9MXioT97SSjxeug0oTXHaKCnQ7txoxuN2+nNdr03mUu07TOUbRp
X3NsiaoESl/9IDC/tz2XTBD3UxLze9pX9t4tdKEMK2+gdnrnioOw1D7WBoElECj9
+89CRXlu3K15P1cNVB5htPzOgwIDAQABo38wfTAOBgNVHQ8BAf8EBAMCBaAwHQYD
VR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwHwYDVR0j
BBgwFoAUO2IRSDwqgkZt5pkXdScs5BjoULEwHQYDVR0RBBYwFIISVmVsb2NpcmFw
dG9yU2VydmVyMA0GCSqGSIb3DQEBCwUAA4IBAQAhwcTMIdHqeR3FXOUREGjkjzC9
vz+hPdXB6w9CMYDOAsmQojuo09h84xt7jD0iqs/K1WJpLSNV3FG5C0TQXa3PD1l3
SsD5p4FfuqFACbPkm/oy+NA7E/0BZazC7iaZYjQw7a8FUx/P+eKo1S7z7Iq8HfmJ
yus5NlnoLmqb/3nZ7DyRWSo9HApmMdNjB6oJWrupSJajsw4Lsos2aJjkfzkg82W7
aGSh9S6Icn1f78BAjJVLv1QBNlb+yGOhrcUWQHERPEpkb1oZJwkVVE1XCZ1C4tVj
PtlBbpcpPHB/R5elxfo+We6vmC8+8XBlNPFFp8LAAile4uQPVQjqy7k/MZ4W
-----END CERTIFICATE-----`
	TestFrontendPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA9MSMbrFjmZs9bnpkel4vTQIyf+6Bpg60ByC7d6WWfBwvHdF1
Qnfn1JO3Xo6p53I1jPoagt0cZCzd6nwJXJ/3pclprmIOEBSc20pg5E0A/kpwn+bB
oPNSrMF7+2/tDvXP0Lvs/1OqUMjF8pCs6vnSKigaptn+0Et3GpzWjwCghqPcJBOu
EuPQmR3HyHfsdsMooCjuYcRcS9MXioT97SSjxeug0oTXHaKCnQ7txoxuN2+nNdr0
3mUu07TOUbRpX3NsiaoESl/9IDC/tz2XTBD3UxLze9pX9t4tdKEMK2+gdnrnioOw
1D7WBoElECj9+89CRXlu3K15P1cNVB5htPzOgwIDAQABAoIBAGAAy3gLOZ6hBgpU
FR7t3C2fRAFrogxozfHRw9Xc69ZIE67lXdGxSAvX2F9NI5T09c4Stt1HLoCYHH6B
Igbjc3XiNwI/0XY7L37PgItrLI2Q0vXUw3OGnJHH3gIz10472cPsQbuvrCi9Zu6K
ElijnewNCM8Sx+AZCWE1zO4P9+Z2kF9LvWzDwAa643jQ/Dg+S68zCFqjJCVJBGm+
LQxDs6dbArvOiEbuZs2wDt0d1kZF+BRljUTMoCpdf3jmFj3f0Jc1AFaz1eHG9Gte
XIUpbWmV2ATABSW2kDkVdXx+m/w1r9PZCLLfq54fIOlm2IeAiM3rDmM4ZSTUYEPn
mJP03xECgYEA+jS7DiS3bB/MeD+5qsgS07qJhOrX17s/SlamC1dQqz+koJLl98JX
CqyafFmdSz7PK2S2+OOazngwx26Kc3MZFoD9IQ2tuWmwDgbY8EQs5Cs37By2YRZJ
DdjvVf48pCKiXxIhvFjW/5CTemNAAu4CXg5Lkp7UVVrOmf5BmjMmE0sCgYEA+m+U
QMF0f7KLM4MU81yAMJdG4Sq4s9i4RmXes2FOUd4UoG7vEpycMKkmEaqiUVmRHPjp
P6Dwq3CK+FVFMpCeWjn6KkxwpdWWO9lglI0npFcPNW/PzPOv4mSNtCAcpHrKFP0R
3jbc8UhgtFxDZoeUih7cO2iTO7kELBCeKUzw9qkCgYBgVYcj1e0tWzztm5OP9sKQ
9MRYAdei/zxKEfySZ0bu+G0ZShXzA8dhm71LXXGbdA5t5bQxNej3z/zv/FagRtOE
/5r2a/7UYaXgcLB8KbOjEiTQ6ukpjlwIUdssn9uXUqJzulZ03zvAYFj4CVivCBav
Qg/E3xRf3LupPOTjSwhA6wKBgQDAH3tnlkHueSWLNiOLc0owfM12jhS2fCsabqpD
iQHRkoLWdWRZLeYw+oLnCLWPnRvTUy11j90yWJt0Wc5FNWcWJuZBLvU4c7vWXDRY
olVoIRXc09NiEwy6rJN9PSlcEYsYQPFFPWeQfwsZMrLOZHLS50vjE53oMk7+Ex2S
56DwSQKBgQC+iHbsbxloZjVMy01V21Sh9RwIpYrodEmwlTZf2jzaYloPadHu4MX1
jHG+zzeC/EJ3wFOKTSJ/Tmjo6N3Xaq9V7WeL8eBdtBtPztqN1yveTt94mZZ+fuID
BhI8P2RbNR2Yey5nnhFQcoTxpmVw3EYwE01nkxoPJRs/QVvxi9Mepg==
-----END RSA PRIVATE KEY-----`
)

func (self *TestSuite) TestCreateAndImportCollection() {
	defer utils.SetFlowIdForTests("F.1234")()
	defer utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))()

	fs_factory := file_store_accessor.NewFileStoreFileSystemAccessor(self.ConfigObj)
	accessors.Register(fs_factory)

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	acl_manager := acl_managers.NullACLManager{}

	// Now create a download of this collection.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := self.Ctx
	scope := manager.BuildScope(builder)
	client_id := "server"

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx, self.ConfigObj,
		acl_manager, repository, &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact", "AnotherTestArtifact"},
			Creator:   utils.GetSuperuserName(self.ConfigObj),
			ClientId:  client_id,
		}, utils.SyncCompleter)
	assert.NoError(self.T(), err)

	// Wait here until the collection is completed.
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		flow, err := launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			"server", flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.State == flows_proto.ArtifactCollectorContext_FINISHED
	})

	// Now create the download export. The plugin returns a filestore
	// pathspec to the created download file.
	result := (&downloads.CreateFlowDownload{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", client_id).
			Set("flow_id", flow_id).
			Set("wait", true))

	download_pathspec, ok := result.(path_specs.FSPathSpec)
	assert.True(self.T(), ok)
	assert.NotEmpty(self.T(), download_pathspec.String())

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	golden := ordereddict.NewDict().
		Set("Original Flow", self.snapshotHuntFlow())

	// Now delete the old flow
	for _ = range (&flows.DeleteFlowPlugin{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", client_id).
			Set("flow_id", flow_id).
			Set("really_do_it", true)) {
	}

	golden.Set("Deleted Flow", self.snapshotHuntFlow())

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	imported_flow := (&collector.ImportCollectionFunction{}).Call(
		ctx, scope, ordereddict.NewDict().
			Set("client_id", client_id).
			Set("filename", download_pathspec).
			Set("accessor", "fs"))
	assert.IsType(self.T(), &flows_proto.ArtifactCollectorContext{}, imported_flow)

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()
	golden.Set("Imported Flow", self.snapshotHuntFlow())

	goldie.Assert(self.T(), "TestCreateAndImportCollection", json.MustMarshalIndent(golden))
}

func (self *TestSuite) TestImportCollectionFromFixture() {
	self.CreateFlow("server", "F.1234")
	defer utils.SetFlowIdForTests("F.1234")()

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)
	_, err := repository.LoadYaml(CustomTestArtifactDependent,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: true})

	assert.NoError(self.T(), err)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := self.Ctx
	scope := manager.BuildScope(builder)

	// This file contains an client_info.json file with the real client's
	// HostID and Hostname.
	import_file_path, err := filepath.Abs("fixtures/import.zip")
	assert.NoError(self.T(), err)

	result := collector.ImportCollectionFunction{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", "auto").

			// This will be ingnored as the new client will be added
			// with the TestHost hostname in the host.json file.
			Set("hostname", "MyNewHost").
			Set("filename", import_file_path))
	context, ok := result.(*proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)

	// Check the import was successful.
	assert.Equal(self.T(), []string{"Linux.Search.FileFinder"},
		context.ArtifactsWithResults)
	assert.Equal(self.T(), uint64(1), context.TotalCollectedRows)
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_FINISHED,
		context.State)

	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Check the indexes are correct for the new client_id
	search_resp, err := indexer.SearchClients(ctx, self.ConfigObj,
		&api_proto.SearchClientsRequest{Query: "host:TestHost"}, "")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 1, len(search_resp.Items))

	new_client_id := search_resp.Items[0].ClientId

	// There is one hit - a new client is added to the index.
	assert.Equal(self.T(), 1, len(search_resp.Items))
	assert.Equal(self.T(), new_client_id, context.ClientId)

	// Importing the collection again and providing the same host name
	// will reuse the client id
	result2 := collector.ImportCollectionFunction{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", "auto").
			Set("hostname", "MyNewHost").
			Set("filename", import_file_path))
	context2, ok := result2.(*proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)

	// The new flow was created on the same client id as before.
	assert.Equal(self.T(), context2.ClientId, context.ClientId)
	assert.Equal(self.T(), context2.ClientId, new_client_id)

	// Importing the collection with a fixed client id will use that client id.
	spec_client_id := "C.1XYZ23"
	result3 := collector.ImportCollectionFunction{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", spec_client_id).
			Set("filename", import_file_path))
	context3, ok := result3.(*proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)

	assert.Equal(self.T(), context3.ClientId, spec_client_id)

	// Now ensure the uploads file is properly adjusted to refer to
	// the client's file store.
	golden := ordereddict.NewDict()
	self.getData("/clients/%s/collections/F.1234/uploads.json",
		"UploadMetadata", golden, new_client_id)

	self.getData("/clients/%s/collections/F.1234/uploads/file/tmp/ls%%5Cwith%%5Cback%%3Aslash",
		"ls\\with\\back\\slash:", golden, new_client_id)

	goldie.Assert(self.T(), "TestImportCollectionFromFixture",
		json.MustMarshalIndent(golden))
}

func (self *TestSuite) TestImportX509CollectionFromFixture() {
	manager, _ := services.GetRepositoryManager(self.ConfigObj)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := self.Ctx
	scope := manager.BuildScope(builder)

	import_file_path, err := filepath.Abs("fixtures/offline_encrypted.zip")
	assert.NoError(self.T(), err)

	result := collector.ImportCollectionFunction{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", "auto").
			Set("hostname", "MyNewHost").
			Set("filename", import_file_path))
	context, ok := result.(*proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)

	assert.Equal(self.T(), []string{"Demo.Plugins.GUI"},
		context.ArtifactsWithResults)
	assert.Equal(self.T(), uint64(1), context.TotalCollectedRows)
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_FINISHED,
		context.State)
}

func (self *TestSuite) getData(
	path string,
	field string,
	golden *ordereddict.Dict,
	new_client_id string) {

	upload_data, pres := test_utils.GetMemoryFileStore(
		self.T(), self.ConfigObj).Get(fmt.Sprintf(path, new_client_id))
	assert.True(self.T(), pres)

	golden.Set(field,
		strings.ReplaceAll(string(upload_data),
			new_client_id, "<client_id>"))

}
