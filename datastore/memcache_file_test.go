package datastore_test

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

var (
	file_based_imp = &datastore.FileBaseDataStore{}
)

type MemcacheFileTestSuite struct {
	BaseTestSuite

	dirname string
	cancel  func()
	ctx     context.Context
	wg      sync.WaitGroup
}

func (self *MemcacheFileTestSuite) SetupTest() {
	// Make a tempdir
	var err error
	self.dirname, err = tempfile.TempDir("datastore_test")
	assert.NoError(self.T(), err)

	self.config_obj = config.GetDefaultConfig()
	self.config_obj.Datastore.Implementation = "MemcacheFileDataStore"
	self.config_obj.Datastore.MemcacheWriteMutationBuffer = 1000
	self.config_obj.Datastore.FilestoreDirectory = self.dirname
	self.config_obj.Datastore.Location = self.dirname
	self.BaseTestSuite.config_obj = self.config_obj

	self.ctx, self.cancel = context.WithCancel(context.Background())

	// Clear the cache between runs
	db := datastore.NewMemcacheFileDataStore(self.ctx, self.config_obj)
	self.datastore = db

	db.Clear()
	db.StartWriter(self.ctx, &self.wg, self.config_obj)
}

func (self *MemcacheFileTestSuite) TearDownTest() {
	self.cancel()
	self.wg.Wait()
	os.RemoveAll(self.dirname) // clean up
}

func (self MemcacheFileTestSuite) TestSetOnFileSystem() {
	_, ok := self.datastore.(*datastore.MemcacheFileDataStore)
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

// Check that the cache works even for very large directories.
func (self MemcacheFileTestSuite) TestDirectoryOverflow() {

	// Expire directories larger than 2 items.
	self.config_obj.Datastore.MemcacheDatastoreMaxDirSize = 4

	db := datastore.NewMemcacheFileDataStore(self.ctx, self.config_obj)
	db.StartWriter(self.ctx, &self.wg, self.config_obj)

	client_record := &api_proto.ClientMetadata{
		ClientId: "C.1234",
	}

	snapshot := vtesting.GetMetrics(self.T(), "memcache_lru_dir_miss")

	intermediate := path_specs.NewSafeDatastorePath("a", "b").
		SetType(api.PATH_TYPE_DATASTORE_DIRECTORY)

	getChildren := func() []string {
		children, err := db.ListChildren(self.config_obj, intermediate)
		assert.NoError(self.T(), err)

		children_str := make([]string, 0, len(children))
		for _, c := range children {
			children_str = append(children_str, c.AsClientPath())
		}

		// Sort for comparison.
		sort.Strings(children_str)
		return children_str
	}

	result := []*ordereddict.Dict{}

	for i := 0; i < 10; i++ {
		urn := path_specs.NewSafeDatastorePath("a", "b", fmt.Sprintf(
			"c%d", i))
		err := db.SetSubject(self.config_obj, urn, client_record)
		assert.NoError(self.T(), err)

		// Make sure that list children is always correct.
		children := getChildren()
		assert.Equal(self.T(), len(children), i+1)

		metrics := vtesting.GetMetricsDifference(
			self.T(), "memcache_lru_dir_miss", snapshot)

		result = append(result, ordereddict.NewDict().
			Set("stats", db.Stats()).
			Set("metrics", metrics))
	}

	// Directory will be cached until there are 4 items in
	// it. DirItemSize is about 40. After that the directory will be
	// marked as full and DirItemSize will be 0. Metrics indicate no
	// misses until it becomes full (except for the first one).

	// Once a directory is deemed too large, then we no longer cache
	// it and every ListChildren() will hit the disk.
	goldie.Assert(self.T(), "TestDirectoryOverflow",
		json.MustMarshalIndent(result))
}

func (self MemcacheFileTestSuite) TestListChildren() {
	_, ok := self.datastore.(*datastore.MemcacheFileDataStore)
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
	assert.Equal(self.T(), len(children), 2)

	// Sort for comparison.
	sort.Slice(children, func(i, j int) bool {
		return children[i].AsClientPath() < children[j].AsClientPath()
	})

	assert.Equal(self.T(), children[0].AsClientPath(), "/a/b")
	assert.Equal(self.T(), children[1].AsClientPath(), "/a/d")
}

func (self MemcacheFileTestSuite) TestSetSubjectAndListChildren() {
	db, ok := self.datastore.(*datastore.MemcacheFileDataStore)
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
	intermediate := path_specs.NewSafeDatastorePath("a", "e", "f")
	new_record := &api_proto.ClientMetadata{}
	err = db.SetSubject(self.config_obj, intermediate, new_record)
	assert.NoError(self.T(), err)

	// Now list the memcache
	first_level := path_specs.NewSafeDatastorePath("a")
	children, err := db.ListChildren(self.config_obj, first_level)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(children), 3)
}

// 1. ListChildren() of /a will cache dir entry
// 2. SetSubject() of /a/e/f/ will implicitly invalidate /a/b/
// 3. ListChildren() of /a will get fresh data.
func (self MemcacheFileTestSuite) TestDeepSetSubjectAfterListChildren() {
	db, ok := self.datastore.(*datastore.MemcacheFileDataStore)
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

	// ListChildren() of /a/ will retrieve from filestore
	first_level := path_specs.NewSafeDatastorePath("a")
	children, err := db.ListChildren(self.config_obj, first_level)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(children), 2)

	// Now set a file in an intermediate directory.
	intermediate := path_specs.NewSafeDatastorePath("a", "e", "f")
	new_record := &api_proto.ClientMetadata{}
	err = db.SetSubject(self.config_obj, intermediate, new_record)
	assert.NoError(self.T(), err)

	// Now list the memcache again should get fresh data.
	children, err = db.ListChildren(self.config_obj, first_level)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(children), 3)
}

func TestMemCacheFileDatastore(t *testing.T) {
	suite.Run(t, &MemcacheFileTestSuite{BaseTestSuite: BaseTestSuite{}})
}
