package acls

import (
	"errors"
	"fmt"

	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func GrantRoles(
	config_obj *config_proto.Config,
	principal string,
	roles []string) error {
	new_policy := &acl_proto.ApiClientACL{}

	for _, role := range roles {
		if !utils.InString(new_policy.Roles, role) {
			if !ValidateRole(role) {
				return errors.New(fmt.Sprintf("Invalid role %v", role))
			}
			new_policy.Roles = append(new_policy.Roles, role)
		}
	}
	return SetPolicy(config_obj, principal, new_policy)
}
