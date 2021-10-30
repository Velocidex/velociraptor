package datastore

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
)

type MemcacheTestSuite struct {
	BaseTestSuite
}

func (self *MemcacheTestSuite) SetupTest() {
	self.datastore.(*MemcacheDatastore).Clear()
}

func TestMemCacheDatastore(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "Memcache"

	suite.Run(t, &MemcacheTestSuite{BaseTestSuite{
		datastore:  NewMemcacheDataStore(),
		config_obj: config_obj,
	}})
}
