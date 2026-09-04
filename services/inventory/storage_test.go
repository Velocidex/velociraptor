package inventory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
)

func TestInventoryStorage(t *testing.T) {
	inventory_service := &InventoryService{}

	ctx := context.Background()
	config_obj := config.GetDefaultConfig()
	for _, v := range []string{"v3-rc1", "v1", "v3"} {
		err := inventory_service.setTool(ctx, config_obj, &artifacts_proto.Tool{
			Name:    "Tool",
			Version: v,
		})
		assert.NoError(t, err)
	}

	// When there is no version specified we pick the latest.
	tool, err := inventory_service.getTool(ctx, config_obj, "Tool", "")
	assert.NoError(t, err)
	assert.Equal(t, tool.Version, "v3")

	// When we specify a version - it returns the exact version.
	tool, err = inventory_service.getTool(ctx, config_obj, "Tool", "v1")
	assert.NoError(t, err)
	assert.Equal(t, tool.Version, "v1")

	// When we specify a version which does not exist, we return an error.
	tool, err = inventory_service.getTool(ctx, config_obj, "Tool", "v4")
	assert.Error(t, err)
}
