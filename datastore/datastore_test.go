package datastore

import (
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	testPaths = []struct {
		urn  api.DSPathSpec
		path string
	}{
		// This one should be safe
		{path_specs.NewSafeDatastorePath("a", "b", "c"), "/a/b/c.db"},

		// Path components are actually a list of strings.
		{path_specs.NewUnsafeDatastorePath("a", "b/c", "d"),
			"/a/b%2Fc/d.db"},

		{path_specs.NewUnsafeDatastorePath("a", "b/c", "d/d"),
			"/a/b%2Fc/d%2Fd.db"},
	}
)

type BaseTestSuite struct {
	suite.Suite

	config_obj *config_proto.Config
	datastore  DataStore
}

func (self BaseTestSuite) TestSetGetJSON() {
	message := &crypto_proto.VeloMessage{Source: "Server"}
	for _, path := range []path_specs.DSPathSpec{
		path_specs.NewUnsafeDatastorePath("a", "b/c", "d"),
		path_specs.NewUnsafeDatastorePath("a", "b/c", "d/a"),
		path_specs.NewUnsafeDatastorePath("a", "b/c", "d?\""),
	} {
		err := self.datastore.SetSubject(
			self.config_obj, path, message)
		assert.NoError(self.T(), err)

		read_message := &crypto_proto.VeloMessage{}
		err = self.datastore.GetSubject(self.config_obj,
			path, read_message)
		assert.NoError(self.T(), err)

		assert.Equal(self.T(), message.Source, read_message.Source)
	}

	// Now test that ListChildren works properly.
	children, err := self.datastore.ListChildren(
		self.config_obj, path_specs.NewUnsafeDatastorePath("a", "b/c"))
	assert.NoError(self.T(), err)

	results := []string{}
	for _, i := range children {
		results = append(results, i.Base())
	}
	sort.Strings(results)
	assert.Equal(self.T(), []string{"d", "d/a", "d?\""}, results)
}

// Old server versions might have data encoded in protobufs. As we
// port new data to json, we need to support reading the old data
// properly.
func (self BaseTestSuite) TestSetGetMigration() {
	message := &crypto_proto.VeloMessage{Source: "Server"}
	for _, path := range []path_specs.DSPathSpec{
		path_specs.NewUnsafeDatastorePath("a", "b", "c"),
	} {
		// Write a protobuf based file
		urn := path_specs.NewSafeDatastorePath(path.Components()...).
			SetType(api.PATH_TYPE_DATASTORE_PROTO)
		err := self.datastore.SetSubject(
			self.config_obj, urn, message)
		assert.NoError(self.T(), err)

		// Even if we read it with json it should work.
		read_message := &crypto_proto.VeloMessage{}
		err = self.datastore.GetSubject(self.config_obj,
			path.SetType(api.PATH_TYPE_DATASTORE_JSON), read_message)
		assert.NoError(self.T(), err)

		assert.Equal(self.T(), message.Source, read_message.Source)
	}
}

func (self BaseTestSuite) TestSetGetSubjectWithEscaping() {
	message := &crypto_proto.VeloMessage{Source: "Server"}
	for _, testcase := range testPaths {
		err := self.datastore.SetSubject(
			self.config_obj, testcase.urn, message)
		assert.NoError(self.T(), err)

		read_message := &crypto_proto.VeloMessage{}
		err = self.datastore.GetSubject(self.config_obj,
			testcase.urn, read_message)
		assert.NoError(self.T(), err)

		assert.Equal(self.T(), message.Source, read_message.Source)
	}
}

func (self BaseTestSuite) TestSetGetSubject() {
	message := &crypto_proto.VeloMessage{Source: "Server"}

	urn := path_specs.NewSafeDatastorePath("a", "b", "c").
		SetType(api.PATH_TYPE_DATASTORE_PROTO)
	err := self.datastore.SetSubject(self.config_obj, urn, message)
	assert.NoError(self.T(), err)

	read_message := &crypto_proto.VeloMessage{}
	err = self.datastore.GetSubject(self.config_obj, urn, read_message)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), message.Source, read_message.Source)

	// Not existing urn returns os.ErrNotExist error and an empty message
	read_message.SessionId = "X"
	err = self.datastore.GetSubject(self.config_obj,
		urn.AddChild("foo"), read_message)
	assert.Error(self.T(), err, os.ErrNotExist)

	// Same for json files.
	read_message.SessionId = "X"
	err = self.datastore.GetSubject(
		self.config_obj, urn.AddChild("foo").
			SetType(api.PATH_TYPE_DATASTORE_JSON),
		read_message)
	assert.Error(self.T(), err, os.ErrNotExist)

	// Delete the subject
	err = self.datastore.DeleteSubject(self.config_obj, urn)
	assert.NoError(self.T(), err)

	// It should now be cleared
	err = self.datastore.GetSubject(self.config_obj, urn, read_message)
	assert.Error(self.T(), err, os.ErrNotExist)
}

func (self BaseTestSuite) TestListChildren() {
	message := &crypto_proto.VeloMessage{Source: "Server"}

	urn := path_specs.NewSafeDatastorePath("a", "b", "c")
	err := self.datastore.SetSubject(self.config_obj,
		urn.AddChild("1"), message)
	assert.NoError(self.T(), err)

	time.Sleep(10 * time.Millisecond)

	err = self.datastore.SetSubject(self.config_obj,
		urn.AddChild("2"), message)
	assert.NoError(self.T(), err)

	time.Sleep(10 * time.Millisecond)

	err = self.datastore.SetSubject(self.config_obj,
		urn.AddChild("3"), message)
	assert.NoError(self.T(), err)

	time.Sleep(10 * time.Millisecond)

	children, err := self.datastore.ListChildren(
		self.config_obj, urn)
	assert.NoError(self.T(), err)

	// ListChildren gives the full path to all children
	assert.Equal(self.T(), []string{
		"/a/b/c/1",
		"/a/b/c/2",
		"/a/b/c/3"}, asStrings(children))

	visited := []api.DSPathSpec{}
	self.datastore.Walk(self.config_obj,
		path_specs.NewSafeDatastorePath("a", "b"),
		func(path_name api.DSPathSpec) error {
			visited = append(visited, path_name)
			return nil
		})
	assert.Equal(self.T(), []string{
		"/a/b/c/1",
		"/a/b/c/2",
		"/a/b/c/3"},
		asStrings(visited))
}

func (self BaseTestSuite) TestUnsafeListChildren() {
	message := &crypto_proto.VeloMessage{Source: "Server"}

	root := path_specs.NewSafeDatastorePath("a")
	urn := root.AddUnsafeChild("b:b", "c:b")
	err := self.datastore.SetSubject(self.config_obj,
		urn.AddChild("1"), message)
	assert.NoError(self.T(), err)

	time.Sleep(10 * time.Millisecond)

	err = self.datastore.SetSubject(self.config_obj,
		urn.AddChild("2"), message)
	assert.NoError(self.T(), err)

	time.Sleep(10 * time.Millisecond)

	err = self.datastore.SetSubject(self.config_obj,
		urn.AddChild("3"), message)
	assert.NoError(self.T(), err)

	time.Sleep(10 * time.Millisecond)

	children, err := self.datastore.ListChildren(
		self.config_obj, urn)
	assert.NoError(self.T(), err)

	// ListChildren gives the full path to all children
	assert.Equal(self.T(), []string{
		"/a/b:b/c:b/1",
		"/a/b:b/c:b/2",
		"/a/b:b/c:b/3"}, asStrings(children))

	visited := []api.DSPathSpec{}
	self.datastore.Walk(self.config_obj,
		root,
		func(path_name api.DSPathSpec) error {
			visited = append(visited, path_name)
			return nil
		})

	assert.Equal(self.T(), []string{
		"/a/b:b/c:b/1",
		"/a/b:b/c:b/2",
		"/a/b:b/c:b/3"},
		asStrings(visited))
}

func (self BaseTestSuite) TestListChildrenSubdirs() {
	message := &crypto_proto.VeloMessage{Source: "Server"}

	urn := path_specs.NewSafeDatastorePath("Root")

	// Add a deep item with the same path as a shorter item.
	err := self.datastore.SetSubject(self.config_obj,
		urn.AddChild("Subdir1", "item"), message)
	assert.NoError(self.T(), err)

	err = self.datastore.SetSubject(self.config_obj,
		urn.AddChild("Subdir1"), message)
	assert.NoError(self.T(), err)

	err = self.datastore.SetSubject(self.config_obj,
		urn.AddChild("item"), message)
	assert.NoError(self.T(), err)

	children, err := self.datastore.ListChildren(
		self.config_obj, urn)
	assert.NoError(self.T(), err)

	// Get one file and one directory
	assert.Equal(self.T(), []string{
		"/Root/Subdir1",
		"/Root/Subdir1:dir",
		"/Root/item"}, asStrings(children))
}

// This test is testing the deprecated index which is still used for
// various things but not for clients any more. Eventually this test
// will be removed.
func (self BaseTestSuite) TestIndexes() {
	client_id := "C.1234"
	client_id_2 := "C.1235"
	err := self.datastore.SetIndex(self.config_obj,
		paths.CLIENT_INDEX_URN_DEPRECATED,
		client_id, []string{"all", client_id, "Hostname", "FQDN", "host:Foo"})
	assert.NoError(self.T(), err)
	err = self.datastore.SetIndex(self.config_obj,
		paths.CLIENT_INDEX_URN_DEPRECATED,
		client_id_2, []string{"all", client_id_2, "Hostname2", "FQDN2", "host:Bar"})
	assert.NoError(self.T(), err)

	hits := self.datastore.SearchClients(self.config_obj,
		paths.CLIENT_INDEX_URN_DEPRECATED, "all", "", 0, 100, SORT_UP)
	sort.Strings(hits)
	assert.Equal(self.T(), []string{client_id, client_id_2}, hits)

	hits = self.datastore.SearchClients(self.config_obj,
		paths.CLIENT_INDEX_URN_DEPRECATED,
		"*foo", "", 0, 100, SORT_UP)
	assert.Equal(self.T(), []string{client_id}, hits)
}

func (self BaseTestSuite) TestQueueMessages() {
	client_id := "C.1236"

	message1 := &crypto_proto.VeloMessage{Source: "Server", SessionId: "1"}
	err := self.datastore.QueueMessageForClient(self.config_obj, client_id, message1)
	assert.NoError(self.T(), err)

	// Now retrieve all messages.
	tasks, err := self.datastore.GetClientTasks(
		self.config_obj, client_id, true /* do_not_lease */)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(tasks))
	assert.True(self.T(), proto.Equal(tasks[0], message1))

	// We did not lease, so the tasks are not removed, but this
	// time we will lease.
	tasks, err = self.datastore.GetClientTasks(
		self.config_obj, client_id, false /* do_not_lease */)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks), 1)

	// No tasks available.
	tasks, err = self.datastore.GetClientTasks(
		self.config_obj, client_id, false /* do_not_lease */)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks), 0)
}

func (self BaseTestSuite) TestFastQueueMessages() {
	client_id := "C.1235"

	written := []*crypto_proto.VeloMessage{}

	for i := 0; i < 10; i++ {
		message := &crypto_proto.VeloMessage{Source: "Server", SessionId: fmt.Sprintf("%d", i)}
		err := self.datastore.QueueMessageForClient(self.config_obj, client_id, message)
		assert.NoError(self.T(), err)

		written = append(written, message)
	}

	// Now retrieve all messages.
	tasks, err := self.datastore.GetClientTasks(
		self.config_obj, client_id, true /* do_not_lease */)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 10, len(tasks))

	// Does not have to return in sorted form.
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].SessionId < tasks[j].SessionId
	})

	for i := 0; i < 10; i++ {
		assert.True(self.T(), proto.Equal(tasks[i], written[i]))
	}
}

func benchmarkSearchClient(b *testing.B,
	data_store DataStore,
	config_obj *config_proto.Config) {

}

func asStrings(in []api.DSPathSpec) []string {
	children := make([]string, 0, len(in))
	for _, i := range in {
		name := utils.JoinComponents(i.Components(), "/")
		if i.IsDir() {
			name += ":dir"
		}

		children = append(children, name)
	}
	sort.Strings(children)

	return children
}
