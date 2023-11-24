package inventory_test

import (
	"sync"

	"github.com/stretchr/testify/assert"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
)

func (self *ServicesTestSuite) TestDummyInventory() {
	wg := &sync.WaitGroup{}
	dummy, err := inventory.NewInventoryDummyService(self.Ctx, wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	// Add some tools to the inventory
	err = dummy.AddTool(self.Ctx, self.ConfigObj, &artifacts_proto.Tool{
		Name: "SampleTool",
		Hash: "SAMLPLEXXXXX",
	}, services.ToolOptions{AdminOverride: true})
	assert.NoError(self.T(), err)

	err = dummy.AddTool(self.Ctx, self.ConfigObj, &artifacts_proto.Tool{
		Name:    "VersionedTool",
		Hash:    "VERSION1XXXXX",
		Version: "1",
	}, services.ToolOptions{AdminOverride: true})
	assert.NoError(self.T(), err)

	err = dummy.AddTool(self.Ctx, self.ConfigObj, &artifacts_proto.Tool{
		Name:    "VersionedTool",
		Hash:    "VERSION2YYYYY",
		Version: "2",
	}, services.ToolOptions{AdminOverride: true})
	assert.NoError(self.T(), err)

	// Now get those tools back out
	tool, err := dummy.GetToolInfo(
		self.Ctx, self.ConfigObj, "SampleTool", "")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), "SAMLPLEXXXXX", tool.Hash)

	// When no version is specified pick the first defined.
	tool, err = dummy.GetToolInfo(
		self.Ctx, self.ConfigObj, "VersionedTool", "")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), "VERSION1XXXXX", tool.Hash)

	// Otherwise when a specific version is specified pick that one
	tool, err = dummy.GetToolInfo(
		self.Ctx, self.ConfigObj, "VersionedTool", "1")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "VERSION1XXXXX", tool.Hash)

	tool, err = dummy.GetToolInfo(
		self.Ctx, self.ConfigObj, "VersionedTool", "2")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), "VERSION2YYYYY", tool.Hash)
}
