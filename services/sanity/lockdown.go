package sanity

import (
	"context"
	"fmt"

	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

func (self SanityChecks) CheckForLockdown(
	ctx context.Context, config_obj *config_proto.Config) error {
	if !config_obj.Lockdown {
		return nil
	}

	lockdown_token := &acl_proto.ApiClientACL{
		ArtifactWriter: true,

		// Labeling clients can move them between label groups which
		// may cause new artifacts to be collected automatically
		// (e.g. Quarantine).
		LabelClients:         true,
		ServerArtifactWriter: true,
		CollectClient:        true,
		CollectServer:        true,
		StartHunt:            true,
		Execve:               true,
		ServerAdmin:          true,
		FilesystemWrite:      true,
		FilesystemRead:       true,
		MachineState:         true,
		CollectBasic:         true,

		Impersonation: true,
		OrgAdmin:      true,
	}

	if config_obj.Security != nil &&
		len(config_obj.Security.LockdownDeniedPermissions) > 0 {
		lockdown_token = &acl_proto.ApiClientACL{}
		for _, perm_name := range config_obj.Security.LockdownDeniedPermissions {
			err := acls.SetTokenPermission(lockdown_token, perm_name)
			if err != nil {
				return fmt.Errorf("Invalid permission %v while parsing lockdown_denied_permissions",
					perm_name)
			}
		}
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<red>Server is in lockdown!</> The following permissions are denied: %v",
		acls.DescribePermissions(lockdown_token))

	acls.SetLockdownToken(lockdown_token)
	return nil
}
