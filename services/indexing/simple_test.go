package indexing_test

import (
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *TestSuite) TestSimpleIndex() {
	client_id := "C.1234"

	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = indexer.SetSimpleIndex(self.ConfigObj,
		paths.CLIENT_INDEX_URN_DEPRECATED,
		client_id, []string{"all", client_id, "Hostname", "FQDN", "host:Foo"})
	assert.NoError(self.T(), err)

	assert.NoError(self.T(), indexer.CheckSimpleIndex(self.ConfigObj,
		paths.CLIENT_INDEX_URN_DEPRECATED, client_id, []string{"Hostname"}))
}
