package api

import (
	"context"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/services"
)

// Raw Datastore access requires the DATASTORE_ACCESS permission. This
// is not provided by any role so the only callers allowed are
// server-server gRPC calls (e.g. minion -> master)
func (self *ApiServer) GetSubject(
	ctx context.Context,
	in *api_proto.DataRequest) (*api_proto.DataResponse, error) {

	defer Instrument("GetSubject")()

	org_config_obj, err := checkDataStorePermissions(ctx, in.OrgId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	db, err := datastore.GetDB(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	raw_db, ok := db.(datastore.RawDataStore)
	if !ok {
		return nil, status.Error(codes.Internal,
			"Datastore has no raw access.")
	}

	data, err := raw_db.GetBuffer(org_config_obj, getURN(in))
	return &api_proto.DataResponse{
		Data: data,
	}, Status(self.verbose, err)
}

func (self *ApiServer) SetSubject(
	ctx context.Context,
	in *api_proto.DataRequest) (*api_proto.DataResponse, error) {

	defer Instrument("SetSubject")()

	org_config_obj, err := checkDataStorePermissions(ctx, in.OrgId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	db, err := datastore.GetDB(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	raw_db, ok := db.(datastore.RawDataStore)
	if !ok {
		return nil, status.Error(codes.Internal,
			"Datastore has no raw access.")
	}

	if in.Sync {
		var wg sync.WaitGroup

		// Wait for the data to hit the disk.
		wg.Add(1)
		err = raw_db.SetBuffer(org_config_obj, getURN(in), in.Data, func() {
			wg.Done()
		})
		wg.Wait()

	} else {

		// Just write quickly.
		err = raw_db.SetBuffer(org_config_obj, getURN(in), in.Data, nil)
	}
	return &api_proto.DataResponse{}, Status(self.verbose, err)
}

func (self *ApiServer) ListChildren(
	ctx context.Context,
	in *api_proto.DataRequest) (*api_proto.ListChildrenResponse, error) {

	defer Instrument("ListChildren")()

	org_config_obj, err := checkDataStorePermissions(ctx, in.OrgId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	db, err := datastore.GetDB(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	children, err := db.ListChildren(org_config_obj, getURN(in))
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result := &api_proto.ListChildrenResponse{}
	for _, child := range children {
		result.Children = append(result.Children, &api_proto.DSPathSpec{
			Components: child.Components(),
			PathType:   int64(child.Type()),
			Tag:        child.Tag(),
			IsDir:      child.IsDir(),
		})
	}

	return result, nil
}

func (self *ApiServer) DeleteSubject(
	ctx context.Context,
	in *api_proto.DataRequest) (*emptypb.Empty, error) {

	defer Instrument("DeleteSubject")()

	org_config_obj, err := checkDataStorePermissions(ctx, in.OrgId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	db, err := datastore.GetDB(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return &emptypb.Empty{}, db.DeleteSubject(org_config_obj, getURN(in))
}

func getURN(in *api_proto.DataRequest) api.DSPathSpec {
	path_spec := in.Pathspec
	if path_spec == nil {
		path_spec = &api_proto.DSPathSpec{}
	}

	return path_specs.NewUnsafeDatastorePath(
		path_spec.Components...).
		SetType(api.PathType(path_spec.PathType)).
		SetTag(path_spec.Tag)
}

func checkDataStorePermissions(
	ctx context.Context, org_id string,
	alternative_permissions ...acls.ACL_PERMISSION) (*config_proto.Config, error) {
	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Shortcut for the superuser - this is the most common method to
	// access the remote datastore.
	if user_record.UserIsSuperuser {
		return org_config_obj, nil
	}

	user_name := user_record.Name
	token, err := services.GetEffectivePolicy(org_config_obj, user_name)
	if err != nil {
		return nil, err
	}

	err = checkToken(token, alternative_permissions)
	if err != nil {
		return nil, err
	}

	// Only the superuser is allowed to switch orgs.
	if token.SuperUser && org_config_obj.OrgId != org_id {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return nil, err
		}

		org_config_obj, err = org_manager.GetOrgConfig(org_id)
		if err != nil {
			return nil, err
		}
	}

	return org_config_obj, nil
}

func checkToken(token *acl_proto.ApiClientACL,
	alternate_permissions []acls.ACL_PERMISSION) error {

	perm, err := services.CheckAccessWithToken(token, acls.DATASTORE_ACCESS)
	if perm && err == nil {
		return nil
	}

	// Check any of the alternate_permissions
	for _, p := range alternate_permissions {
		perm, err := services.CheckAccessWithToken(token, p)
		if perm && err == nil {
			return nil
		}
	}
	return PermissionDenied(err,
		"User is not allowed to access datastore.")
}
