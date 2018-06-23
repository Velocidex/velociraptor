package api

import (
	"google.golang.org/grpc/metadata"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config "www.velocidex.com/golang/velociraptor/config"
)

// TODO: Implement this properly.
func IsUserApprovedForClient(
	config_obj *config.Config,
	md *metadata.MD,
	client_id string) bool {
	return true
}

func getClientApprovalForUser(
	config *config.Config,
	md *metadata.MD,
	client_id string) *api_proto.ApprovalList {
	result := api_proto.ApprovalList{
		Items: []*api_proto.Approval{
			{Reason: "test"},
		},
	}

	return &result
}

func NewDefaultUserObject() *api_proto.ApiGrrUser {
	return &api_proto.ApiGrrUser{
		InterfaceTraits: &api_proto.ApiGrrUserInterfaceTraits{
			CronJobsNavItemEnabled:                true,
			CreateCronJobActionEnabled:            true,
			HuntManagerNavItemEnabled:             true,
			CreateHuntActionEnabled:               true,
			ShowStatisticsNavItemEnabled:          true,
			ServerLoadNavItemEnabled:              true,
			ManageBinariesNavItemEnabled:          true,
			UploadBinaryActionEnabled:             true,
			SettingsNavItemEnabled:                true,
			ArtifactManagerNavItemEnabled:         true,
			UploadArtifactActionEnabled:           true,
			SearchClientsActionEnabled:            true,
			BrowseVirtualFileSystemNavItemEnabled: true,
			StartClientFlowNavItemEnabled:         true,
			ManageClientFlowsNavItemEnabled:       true,
			ModifyClientLabelsActionEnabled:       true,
		},
		UserType: api_proto.ApiGrrUser_USER_TYPE_ADMIN,
	}
}
