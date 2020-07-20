package api

import (
	"github.com/golang/protobuf/ptypes/empty"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	users "www.velocidex.com/golang/velociraptor/users"
)

func (self *ApiServer) GetUsers(
	ctx context.Context,
	in *empty.Empty) (*api_proto.Users, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to enumerate users.")
	}

	result := &api_proto.Users{}

	users, err := users.ListUsers(self.config)
	if err != nil {
		return nil, err
	}

	result.Users = users

	return result, nil
}
