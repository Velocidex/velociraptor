package vql

import (
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
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

func VQLMetadata() *MetadataBuilder {
	return &MetadataBuilder{ordereddict.NewDict()}
}
