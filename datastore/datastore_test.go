package datastore

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
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
	message := &crypto_proto.GrrMessage{Source: "Server"}

	urn := "/a/b/c"
	err := self.datastore.SetSubject(self.config_obj, urn, message)
	assert.NoError(self.T(), err)

	read_message := &crypto_proto.GrrMessage{}
	err = self.datastore.GetSubject(self.config_obj, urn, read_message)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), message.Source, read_message.Source)

	// Not existing urn returns no error but an empty message
	err = self.datastore.GetSubject(self.config_obj, urn+"foo", read_message)
	assert.NoError(self.T(), err)

	// Same for json files.
	err = self.datastore.GetSubject(
		self.config_obj, urn+"foo.json", read_message)
	assert.NoError(self.T(), err)

	// Delete the subject
	err = self.datastore.DeleteSubject(self.config_obj, urn)
	assert.NoError(self.T(), err)

	// It should now be cleared
	err = self.datastore.GetSubject(self.config_obj, urn, read_message)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), "", read_message.Source)
}

func (self BaseTestSuite) TestListChildren() {
	message := &crypto_proto.GrrMessage{Source: "Server"}

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

func benchmarkSearchClient(b *testing.B,
	data_store DataStore,
	config_obj *config_proto.Config) {

}
