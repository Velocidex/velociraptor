package inventory

import (
	"context"

	"google.golang.org/protobuf/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Replace the tool in the inventory or if it is not there already,
// add it.
func (self *InventoryService) setTool(
	ctx context.Context, config_obj *config_proto.Config,
	tool *artifacts_proto.Tool) error {

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	found := false
	for i, item := range self.binaries.Tools {
		if item.Name == tool.Name &&
			item.Version == tool.Version {
			found = true
			self.binaries.Tools[i] = tool
			break
		}
	}

	if !found {
		self.binaries.Tools = append(self.binaries.Tools, tool)
	}

	self.binaries.Version = uint64(utils.GetTime().Now().UnixNano())

	// Ensure the inventory is saved when it changes.
	err := self.saveInventory(ctx, config_obj)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Warn("Unable to store inventory - will run with an in memory one.")
	}

	return nil
}

// Find the best tool to match the version and name.  If a version is
// not specified, we need to sort the tools by semantic version so we
// get the latest version available. If version is specified we return
// the exact match.
func (self *InventoryService) getTool(
	ctx context.Context, config_obj *config_proto.Config,
	tool_name, version string) (tool *artifacts_proto.Tool, err error) {

	// If a version is not specified, we need to sort the tools by
	// semantic version so we get the latest version available.
	var match *artifacts_proto.Tool
	for _, item := range self.binaries.Tools {
		if item.Name != tool_name {
			continue
		}

		// Match an exact version
		if version != "" {
			if version == item.Version {
				match = item
				break
			}
			continue
		}

		// Look for the largest version available
		if match == nil {
			match = item
			continue
		}

		// Match the latest version available.
		if utils.CompareVersions(item.Name, match.Version, item.Version) < 0 {
			match = item
		}
	}

	if match == nil {
		return nil, utils.Wrap(utils.NotFoundError,
			"Tool %v not declared in inventory.", tool)
	}
	return proto.Clone(match).(*artifacts_proto.Tool), nil
}
