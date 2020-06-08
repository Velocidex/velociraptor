package services

import (
	"path"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type VFSServiceTestSuite struct {
	BaseServicesTestSuite
}

func (self *VFSServiceTestSuite) TestVFSListDirectory() {
	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "c"),
			makeStat("/a/b", "d"),
			makeStat("/a/b", "e"),
		})

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	resp := &flows_proto.VFSListResponse{}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.config_obj,
			client_path_manager.VFSPath([]string{"file", "a", "b"}),
			resp)
		return resp.TotalRows == 3
	})
	assert.Equal(self.T(), self.getFullPath(resp), []string{
		"/a/b/c", "/a/b/d", "/a/b/e",
	})
}

func (self *VFSServiceTestSuite) TestRecursiveVFSListDirectory() {
	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "A"),
			makeStat("/a/b", "B"),
			makeStat("/a/b/c", "CA"),
			makeStat("/a/b/c", "CB"),
		})

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	resp := &flows_proto.VFSListResponse{}

	// The response in VFS path /file/a/b
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.config_obj,
			client_path_manager.VFSPath([]string{"file", "a", "b"}),
			resp)
		return resp.TotalRows == 2
	})

	assert.Equal(self.T(), self.getFullPath(resp), []string{
		"/a/b/A", "/a/b/B",
	})

	// The response in VFS path /file/a/b/c
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.config_obj,
			client_path_manager.VFSPath([]string{"file", "a", "b", "c"}),
			resp)
		return resp.TotalRows == 2
	})

	assert.Equal(self.T(), self.getFullPath(resp), []string{
		"/a/b/c/CA", "/a/b/c/CB",
	})
}

func (self *VFSServiceTestSuite) TestVFSDownload() {
	flow_path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id)

	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "A"),
			makeStat("/a/b", "B"),
		})

	// Simulate and upload was received by our System.VFS.DownloadFile collection.
	file_store := self.GetMemoryFileStore()
	file_store.Data[flow_path_manager.GetUploadsFile("file", "/a/b/B").Path()] = []byte("Data")

	self.EmulateCollection(
		"System.VFS.DownloadFile", []*ordereddict.Dict{
			ordereddict.NewDict().
				Set("Path", "/a/b/B").
				Set("Accessor", "file").
				Set("Size", 10),
		})

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(self.T(), err)

	// The VFS service stores a file in the VFS area of the
	// client's namespace pointing to the real data. The real data
	// is stored in the collection's space.
	resp := &proto.VFSDownloadInfo{}
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.config_obj,
			flow_path_manager.GetVFSDownloadInfoPath("file", "/a/b/B").Path(),
			resp)
		return resp.Size == 10
	})

	assert.Equal(self.T(), resp.VfsPath,
		flow_path_manager.GetUploadsFile("file", "/a/b/B").Path())
}

func (self *VFSServiceTestSuite) getFullPath(resp *flows_proto.VFSListResponse) []string {
	rows, err := utils.ParseJsonToDicts([]byte(resp.Response))
	assert.NoError(self.T(), err)

	result := []string{}
	for _, row := range rows {
		full_path, ok := row.GetString("_FullPath")
		if ok {
			result = append(result, full_path)
		}
	}

	return result
}

func makeStat(dirname, name string) *ordereddict.Dict {
	fullpath := path.Join(dirname, name)
	return ordereddict.NewDict().Set("_FullPath", fullpath).
		Set("Name", name).Set("_Accessor", "file")
}

func TestVFSService(t *testing.T) {
	suite.Run(t, &VFSServiceTestSuite{BaseServicesTestSuite{}})
}
