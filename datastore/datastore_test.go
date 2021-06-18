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
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type BaseTestSuite struct {
	suite.Suite

	config_obj *config_proto.Config
	datastore  DataStore
}

func (self BaseTestSuite) TestSetGetSubject() {
	message := &crypto_proto.VeloMessage{Source: "Server"}

	urn := "/a/b/c"
	err := self.datastore.SetSubject(self.config_obj, urn, message)
	assert.NoError(self.T(), err)

	read_message := &crypto_proto.VeloMessage{}
	err = self.datastore.GetSubject(self.config_obj, urn, read_message)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), message.Source, read_message.Source)

	// Not existing urn returns os.ErrNotExist error and an empty message
	read_message.SessionId = "X"
	err = self.datastore.GetSubject(self.config_obj, urn+"foo", read_message)
	assert.Error(self.T(), err, os.ErrNotExist)

	// Same for json files.
	read_message.SessionId = "X"
	err = self.datastore.GetSubject(
		self.config_obj, urn+"foo.json", read_message)
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

	urn := "/a/b/c"
	err := self.datastore.SetSubject(self.config_obj, urn+"/1", message)
	assert.NoError(self.T(), err)

	time.Sleep(10 * time.Millisecond)

	err = self.datastore.SetSubject(self.config_obj, urn+"/2", message)
	assert.NoError(self.T(), err)

	time.Sleep(10 * time.Millisecond)

	err = self.datastore.SetSubject(self.config_obj, urn+"/3", message)
	assert.NoError(self.T(), err)

	time.Sleep(10 * time.Millisecond)

	children, err := self.datastore.ListChildren(self.config_obj, urn, 0, 100)
	assert.NoError(self.T(), err)

	// ListChildren gives the full path to all children
	sort.Strings(children)
	assert.Equal(self.T(), []string{
		"/a/b/c/1",
		"/a/b/c/2",
		"/a/b/c/3"}, children)

	children, err = self.datastore.ListChildren(self.config_obj, urn, 0, 2)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), []string{"/a/b/c/1", "/a/b/c/2"}, children)

	children, err = self.datastore.ListChildren(self.config_obj, urn, 1, 2)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), []string{"/a/b/c/2", "/a/b/c/3"}, children)

	visited := []string{}
	self.datastore.Walk(self.config_obj, "/\"a\"/b",
		func(path_name string) error {
			visited = append(visited, path_name)
			return nil
		})
	sort.Strings(visited)
	assert.Equal(self.T(), []string{"/a/b/c/1", "/a/b/c/2", "/a/b/c/3"}, visited)
}

func (self BaseTestSuite) TestIndexes() {
	client_id := "C.1234"
	client_id_2 := "C.1235"
	err := self.datastore.SetIndex(self.config_obj, constants.CLIENT_INDEX_URN,
		client_id, []string{"all", client_id, "Hostname", "FQDN", "host:Foo"})
	assert.NoError(self.T(), err)
	err = self.datastore.SetIndex(self.config_obj, constants.CLIENT_INDEX_URN,
		client_id_2, []string{"all", client_id_2, "Hostname2", "FQDN2", "host:Bar"})
	assert.NoError(self.T(), err)

	hits := self.datastore.SearchClients(self.config_obj, constants.CLIENT_INDEX_URN,
		"all", "", 0, 100, SORT_UP)
	sort.Strings(hits)
	assert.Equal(self.T(), []string{client_id, client_id_2}, hits)

	hits = self.datastore.SearchClients(self.config_obj, constants.CLIENT_INDEX_URN,
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
