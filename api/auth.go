package api

import (
	"context"
	"encoding/json"
	"net/http"

	"google.golang.org/grpc/metadata"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	users "www.velocidex.com/golang/velociraptor/users"
)

var (
	contextKeyUser = "USER"
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
		if !ok {
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

		// Checking is successfull - user authorized. Here we
		// build a token to pass to the underlying GRPC
		// service with metadata about the user.
		user_info := &api_proto.VelociraptorUser{
			Name: username,
		}

		// Must use json encoding because grpc can not handle
		// binary data in metadata.
		serialized, _ := json.Marshal(user_info)
		ctx := context.WithValue(
			r.Context(), "USER", string(serialized))

		// Need to call logging after auth so it can access
		// the USER value in the context.
		logging.GetLoggingHandler(config_obj)(parent).ServeHTTP(
			w, r.WithContext(ctx))
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

func GetGRPCUserInfo(ctx context.Context) *api_proto.VelociraptorUser {
	result := &api_proto.VelociraptorUser{}

	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		userinfo := md.Get("USER")
		if len(userinfo) > 0 {
			data := []byte(userinfo[0])
			json.Unmarshal(data, result)
		}
	}

	return result
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
