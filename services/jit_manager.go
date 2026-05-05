package services

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

const (
	MaxJITDurationSec = 4 * 60 * 60 // 4 hours
)

type JITManager interface {
	RequestRole(
		config_obj *config_proto.Config,
		requester string,
		request *api_proto.JITRequestRoleRequest) (*api_proto.JITRoleRequest, error)

	ApproveOrDeny(
		config_obj *config_proto.Config,
		approver string,
		approval *api_proto.JITApprovalRequest) (*api_proto.JITRoleRequest, error)

	RevokeGrant(
		config_obj *config_proto.Config,
		principal string,
		request_id string) error

	ListRequests(
		config_obj *config_proto.Config,
		status api_proto.JITRequestStatus,
		username string) (*api_proto.JITRoleRequests, error)

	// Returns active (approved and not expired) grants for a user
	GetActiveGrants(
		config_obj *config_proto.Config,
		username string) ([]*api_proto.JITRoleRequest, error)
}

func GetJITManager(config_obj *config_proto.Config) (JITManager, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}
	return org_manager.Services(config_obj.OrgId).JITManager()
}
