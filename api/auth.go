package api

import (
	"context"
	"net/http"

	"google.golang.org/grpc/metadata"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	users "www.velocidex.com/golang/velociraptor/users"
)

func checkUserCredentialsHandler(
	config_obj *api_proto.Config,
	parent http.Handler) http.Handler {

	// We are supposed to do the oauth thing.
	if config_obj.GUI.GoogleOauthClientId != "" &&
		config_obj.GUI.GoogleOauthClientSecret != "" {
		return authenticateOAUTHCookie(config_obj, parent)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)

		username, password, ok := r.BasicAuth()
		if ok == false {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		user_record, err := users.GetUser(config_obj, username)
		if err != nil {
			http.Error(w, "authorization failed", http.StatusUnauthorized)
			return
		}

		if user_record.Name != username ||
			!user_record.VerifyPassword(password) {
			http.Error(w, "authorization failed", http.StatusUnauthorized)
			return
		}

		// Record the username for handlers lower in the stack.
		ctx := context.WithValue(r.Context(), "USER", username)
		parent.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TODO: Implement this properly.
func IsUserApprovedForClient(
	config_obj *api_proto.Config,
	md *metadata.MD,
	client_id string) bool {
	return true
}

func getClientApprovalForUser(
	config *api_proto.Config,
	md *metadata.MD,
	client_id string) *api_proto.ApprovalList {
	result := api_proto.ApprovalList{
		Items: []*api_proto.Approval{
			{Reason: "test"},
		},
	}

	return &result
}

func getUsername(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		username := md.Get("USER")
		if len(username) > 0 {
			return username[0]
		}
	}
	return ""
}

func NewDefaultUserObject(config_obj *api_proto.Config) *api_proto.ApiGrrUser {
	result := &api_proto.ApiGrrUser{
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
			AuthUsingGoogle:                       config_obj.GUI.GoogleOauthClientId != "",
		},
		UserType: api_proto.ApiGrrUser_USER_TYPE_ADMIN,
	}
	return result
}
