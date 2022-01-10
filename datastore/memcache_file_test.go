package datastore

import (
	"context"
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type MemcacheFileTestSuite struct {
	BaseTestSuite

	dirname string
	cancel  func()
	ctx     context.Context
	wg      sync.WaitGroup
}

func (self *MemcacheFileTestSuite) SetupTest() {
	// Clear the cache between runs
	db := self.datastore.(*MemcacheFileDataStore)

	// Make a tempdir
	var err error
	self.dirname, err = ioutil.TempDir("", "datastore_test")
	assert.NoError(self.T(), err)

	self.config_obj = config.GetDefaultConfig()
	self.config_obj.Datastore.Implementation = "MemcacheFileDataStore"
	self.config_obj.Datastore.MemcacheWriteMutationBuffer = -1
	self.config_obj.Datastore.FilestoreDirectory = self.dirname
	self.config_obj.Datastore.Location = self.dirname
	self.BaseTestSuite.config_obj = self.config_obj

	self.ctx, self.cancel = context.WithCancel(context.Background())

	db.Clear()
	db.StartWriter(self.ctx, &self.wg, self.config_obj)
}

func (self *MemcacheFileTestSuite) TearDownTest() {
	self.cancel()
	self.wg.Wait()
	os.RemoveAll(self.dirname) // clean up
}

func (self MemcacheFileTestSuite) TestSetOnFileSystem() {
	_, ok := self.datastore.(*MemcacheFileDataStore)
	assert.True(self.T(), ok)

	// Setting the data ends up on the filesystem
	client_id := "C.1234"
	client_pathspec := paths.NewClientPathManager(client_id)
	client_record := &api_proto.ClientMetadata{
		ClientId: client_id,
	}
	// Write the file to the memcache and read it from the filesystem
	err := self.datastore.SetSubject(
		self.config_obj, client_pathspec.Path(), client_record)
	assert.NoError(self.T(), err)

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		client_record := &api_proto.ClientMetadata{}
		err := file_based_imp.GetSubject(
			self.config_obj, client_pathspec.Path(), client_record)
		if err != nil {
			return false
		}
		return client_record.ClientId == client_id
	})

	// Now write a file to the filsystem and read it from memcache.
	flow_id := "F.123"
	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	md := &flows_proto.ArtifactCollectorContext{
		SessionId: flow_id,
		ClientId:  client_id,
	}

	err = file_based_imp.SetSubject(self.config_obj,
		flow_path_manager.Path(), md)
	assert.NoError(self.T(), err)

	new_md := &flows_proto.ArtifactCollectorContext{}
	err = self.datastore.GetSubject(
		self.config_obj, flow_path_manager.Path(), new_md)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), new_md.SessionId, md.SessionId)
}

func (self MemcacheFileTestSuite) TestListChildren() {
	_, ok := self.datastore.(*MemcacheFileDataStore)
	assert.True(self.T(), ok)

	// Setting the data ends up on the filesystem
	client_id := "C.1234"
	client_record := &api_proto.ClientMetadata{
		ClientId: client_id,
	}

	// Write the file to the filesystem
	urn := path_specs.NewSafeDatastorePath("a", "b", "c")
	err := file_based_imp.SetSubject(self.config_obj, urn, client_record)
	assert.NoError(self.T(), err)

	urn2 := path_specs.NewSafeDatastorePath("a", "d", "e")
	err = file_based_imp.SetSubject(self.config_obj, urn2, client_record)
	assert.NoError(self.T(), err)

	// Now read a file from memcache - this should refresh internal
	// list directories.
	new_record := &api_proto.ClientMetadata{}
	err = self.datastore.GetSubject(self.config_obj, urn, new_record)
	assert.NoError(self.T(), err)

	// Now list the memcache
	intermediate := path_specs.NewSafeDatastorePath("a")
	children, err := self.datastore.ListChildren(self.config_obj, intermediate)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(children), 1)
	assert.Equal(self.T(), children[0].AsClientPath(), "/a/b")
}

func (self MemcacheFileTestSuite) TestSetSubjectAndListChildren() {
	db, ok := self.datastore.(*MemcacheFileDataStore)
	assert.True(self.T(), ok)

	// Setting the data ends up on the filesystem
	client_id := "C.1234"
	client_record := &api_proto.ClientMetadata{
		ClientId: client_id,
	}

	// Write the file to the filesystem
	urn := path_specs.NewSafeDatastorePath("a", "b")
	err := file_based_imp.SetSubject(self.config_obj, urn, client_record)
	assert.NoError(self.T(), err)

	urn2 := path_specs.NewSafeDatastorePath("a", "d")
	err = file_based_imp.SetSubject(self.config_obj, urn2, client_record)
	assert.NoError(self.T(), err)

	// Now set a file in an existing directory.
	intermediate := path_specs.NewSafeDatastorePath("a", "e")
	new_record := &api_proto.ClientMetadata{}
	err = db.SetSubject(self.config_obj, intermediate, new_record)
	assert.NoError(self.T(), err)

	// Now list the memcache
	first_level := path_specs.NewSafeDatastorePath("a")
	children, err := db.ListChildren(self.config_obj, first_level)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(children), 3)
	utils.Debug(children)
}

func TestMemCacheFileDatastore(t *testing.T) {
	suite.Run(t, &MemcacheFileTestSuite{BaseTestSuite: BaseTestSuite{
		datastore: NewMemcacheFileDataStore(),
	}})
}
