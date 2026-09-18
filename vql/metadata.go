package vql

import (
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
)

// Where is the plugin allowed to run on.
type ExecutionContext string

var (
	// Plugin is only allowed to run on the master node:
	// - Clients will reject the plugin.
	// - Minions will forward calls to the master node.
	MasterExecutionContext ExecutionContext = "master"
)

type MetadataBuilder struct {
	*ordereddict.Dict
}

func (self *MetadataBuilder) Permissions(perms ...acls.ACL_PERMISSION) *MetadataBuilder {
	parts := []string{}
	for _, p := range perms {
		parts = append(parts, p.String())
	}
	self.Set("permissions", strings.Join(parts, ","))
	return self
}

func (self *MetadataBuilder) Build() *ordereddict.Dict {
	return self.Dict
}

// Used to tag plugins or functions that only run on the master node.
func (self *MetadataBuilder) ExecutionContext(
	ctx ...ExecutionContext) *MetadataBuilder {
	parts := []string{}
	for _, p := range ctx {
		parts = append(parts, string(p))
	}
	self.Set("execution", strings.Join(parts, ","))
	return self
}

func VQLMetadata() *MetadataBuilder {
	return &MetadataBuilder{ordereddict.NewDict()}
}
