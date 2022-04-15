package indexing_test

import (
	"github.com/alecthomas/assert"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *TestSuite) TestSimpleIndex() {
	client_id := "C.1234"

	indexer, err := services.GetIndexer()
	assert.NoError(self.T(), err)

	err = indexer.SetSimpleIndex(self.ConfigObj,
		paths.CLIENT_INDEX_URN_DEPRECATED,
		client_id, []string{"all", client_id, "Hostname", "FQDN", "host:Foo"})
	assert.NoError(self.T(), err)

	assert.NoError(self.T(), indexer.CheckSimpleIndex(self.ConfigObj,
		paths.CLIENT_INDEX_URN_DEPRECATED, client_id, []string{"Hostname"}))
}
