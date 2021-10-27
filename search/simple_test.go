package search_test

import (
	"github.com/alecthomas/assert"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/search"
)

func (self *TestSuite) TestSimpleIndex() {
	client_id := "C.1234"

	err := search.SetSimpleIndex(self.ConfigObj,
		paths.CLIENT_INDEX_URN_DEPRECATED,
		client_id, []string{"all", client_id, "Hostname", "FQDN", "host:Foo"})
	assert.NoError(self.T(), err)

	assert.NoError(self.T(), search.CheckSimpleIndex(self.ConfigObj,
		paths.CLIENT_INDEX_URN_DEPRECATED, client_id, []string{"Hostname"}))
}
