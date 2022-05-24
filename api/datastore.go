package api

import (
	"sync"

	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
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

	users := services.GetUserManager()
	user_record, err := users.GetUserFromContext(self.config, ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	perm, err := acls.CheckAccess(self.config, user_name, acls.DATASTORE_ACCESS)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to access datastore.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	raw_db, ok := db.(datastore.RawDataStore)
	if !ok {
		return nil, status.Error(codes.Internal,
			"Datastore has no raw access.")
	}

	data, err := raw_db.GetBuffer(self.config, getURN(in))
	return &api_proto.DataResponse{
		Data: data,
	}, err
}

func (self *ApiServer) SetSubject(
	ctx context.Context,
	in *api_proto.DataRequest) (*api_proto.DataResponse, error) {

	users := services.GetUserManager()
	user_record, err := users.GetUserFromContext(self.config, ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	perm, err := acls.CheckAccess(self.config, user_name, acls.DATASTORE_ACCESS)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to access datastore.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
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
		err = raw_db.SetBuffer(self.config, getURN(in), in.Data, func() {
			wg.Done()
		})
		wg.Wait()

	} else {

		// Just write quickly.
		err = raw_db.SetBuffer(self.config, getURN(in), in.Data, nil)
	}
	return &api_proto.DataResponse{}, err
}

func (self *ApiServer) ListChildren(
	ctx context.Context,
	in *api_proto.DataRequest) (*api_proto.ListChildrenResponse, error) {

	users := services.GetUserManager()
	user_record, err := users.GetUserFromContext(self.config, ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	perm, err := acls.CheckAccess(self.config, user_name, acls.DATASTORE_ACCESS)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to access datastore.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	children, err := db.ListChildren(self.config, getURN(in))
	if err != nil {
		return nil, err
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

	users := services.GetUserManager()
	user_record, err := users.GetUserFromContext(self.config, ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	perm, err := acls.CheckAccess(self.config, user_name, acls.DATASTORE_ACCESS)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to access datastore.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, db.DeleteSubject(self.config, getURN(in))
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
