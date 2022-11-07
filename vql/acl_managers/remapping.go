package acl_managers

import (
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// Get a new, more restricted ACL manager suitable for remapping
// configuration. NOTE that this remapping manager can not give
// **more** permissions than before, but can only remove permissions
// from the existing token. It is useful when we want to block
// certain plugins from working because we are emulating a more
// restricted environment. For example when analyzing a dead image on
// Windows we need to prevent wmi() plugin from interrogating the
// analysis host, therefore would typically remove the MACHINE_STATE
// permission.
func GetRemappingACLManager(
	config_obj *config_proto.Config,
	existing_manager vql_subsystem.ACLManager,
	remap_config []*config_proto.RemappingConfig) (vql_subsystem.ACLManager, error) {
	token := &acl_proto.ApiClientACL{}
	for _, item := range remap_config {
		if item.Type == "permissions" {
			for _, perm := range item.Permissions {
				allowed, err := existing_manager.CheckAccess(
					acls.GetPermission(perm))
				if err == nil && allowed {
					err := acls.SetTokenPermission(token, perm)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}

	return &RoleACLManager{
		Token:      token,
		config_obj: config_obj,
	}, nil
}
