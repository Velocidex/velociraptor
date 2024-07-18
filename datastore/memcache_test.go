package datastore_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/datastore"
)

type MemcacheTestSuite struct {
	BaseTestSuite
}

func (self *MemcacheTestSuite) SetupTest() {
	self.datastore.(*datastore.MemcacheDatastore).Clear()
}

func TestMemCacheDatastore(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "Memcache"

	ctx := context.Background()
	suite.Run(t, &MemcacheTestSuite{BaseTestSuite{
		datastore:  datastore.NewMemcacheDataStore(ctx, config_obj),
		config_obj: config_obj,
	}})
}
