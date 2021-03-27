package datastore

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
)

type MemoryTestSuite struct {
	BaseTestSuite
}

func TestMemoryDatastore(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "Test"

	suite.Run(t, &MemoryTestSuite{BaseTestSuite{
		datastore:  NewTestDataStore(),
		config_obj: config_obj,
	}})
}
