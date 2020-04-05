package artifacts

import (
	"fmt"

	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/acls"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func (self *Repository) CheckAccess(
	config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact, principal string) error {

	if artifact.RequiredPermissions == nil {
		return nil
	}

	// Principal must have ALL permissions to succeed.
	for _, perm := range artifact.RequiredPermissions {
		permission := acls.GetPermission(perm)
		perm, err := acls.CheckAccess(config_obj, principal, permission)
		if !perm || err != nil {
			return errors.New(fmt.Sprintf(
				"User %v is not allowed to collect this artifact (%s)",
				principal, permission))
		}
	}

	return nil
}
