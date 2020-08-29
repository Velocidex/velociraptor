package services

// The Velociraptor server maintains a database of tools. Artifacts
// may simply ask to use a particular tool by name, and if the tool is
// previously known or uploaded, Velociraptor will make that tool
// available to the artifact.

// This service maintains the internal tools database.

import (
	"context"
	"sync"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	inventory_mu sync.Mutex
	ginventory   Inventory
)

func GetInventory() Inventory {
	inventory_mu.Lock()
	defer inventory_mu.Unlock()

	return ginventory
}

func RegisterInventory(inventory Inventory) {
	inventory_mu.Lock()
	defer inventory_mu.Unlock()

	ginventory = inventory
}

type ToolOptions struct {
	// Tool is being upgraded.
	Upgrade bool

	// Admin is overriding tool in inventory.
	AdminOverride bool
}

type Inventory interface {
	// Get a list of the entire tools database with all known tools.
	Get() *artifacts_proto.ThirdParty

	// Probe for a specific tool without materializing the tool.
	ProbeToolInfo(name string) (*artifacts_proto.Tool, error)

	// Get information about a specific tool. If the tool is set
	// to serve locally, the tool will be fetched from its
	// designated URL. This call will materialize the tool and
	// update the state fields  (e.g. serve_url, filestore_path,
	// filename, hash)
	GetToolInfo(ctx context.Context, config_obj *config_proto.Config,
		tool string) (*artifacts_proto.Tool, error)

	// Add a new tool to the inventory. Adding the tool does not
	// force it to be downloaded - it simply adds it to the
	// database and does not block. A subsequent GetToolInfo()
	// will download the tool from the designated URL if the hash
	// is not already known. If callers need to ensure the tool is
	// actually valid and available, they need to call
	// GetToolInfo() after this to force the tool to be
	// materialized.
	AddTool(config_obj *config_proto.Config,
		tool *artifacts_proto.Tool, opts ToolOptions) error

	// Remove the tool from the inventory.
	RemoveTool(config_obj *config_proto.Config, tool_name string) error
}
