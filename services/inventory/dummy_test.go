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

func (self *ServicesTestSuite) TestDummyInventoryReadWrite() {
	wg := &sync.WaitGroup{}
	dummy, err := inventory.NewInventoryDummyService(self.Ctx, wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	// Add some tools to the inventory
	err = dummy.AddTool(self.Ctx, self.ConfigObj, &artifacts_proto.Tool{
		Name: "SampleTool",
	}, services.ToolOptions{AdminOverride: true})
	assert.NoError(self.T(), err)

	// Write the data
	data := "Hello world"
	hash_str := getHash(data)

	writer, err := dummy.WriteTool(self.Ctx,
		self.ConfigObj, "SampleTool", "")
	assert.NoError(self.T(), err)

	_, err = writer.Write([]byte(data))
	assert.NoError(self.T(), err)
	assert.NoError(self.T(), writer.Close())

	tool, err := dummy.GetToolInfo(self.Ctx, self.ConfigObj, "SampleTool", "")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), hash_str, tool.Hash)

	fd, err := dummy.ReadTool(self.Ctx, self.ConfigObj, "SampleTool", "")
	assert.NoError(self.T(), err)

	buffer := make([]byte, 100)
	n, err := fd.Read(buffer)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), string(buffer[:n]), string(data))
}
