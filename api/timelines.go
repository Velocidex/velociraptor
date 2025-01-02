package api

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func (self *ApiServer) AnnotateTimeline(
	ctx context.Context,
	in *api_proto.AnnotationRequest) (*emptypb.Empty, error) {

	defer Instrument("AnnotateTimeline")()

	// Empty creators are called internally.
	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	principal := user_record.Name

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err, "User is not allowed to update notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	scope := vql_subsystem.MakeScope()
	event := ordereddict.NewDict()
	err = json.Unmarshal([]byte(in.EventJson), &event)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	err = notebook_manager.AnnotateTimeline(ctx, scope, in.NotebookId,
		in.SuperTimeline, in.Note, principal, time.Unix(0, in.Timestamp),
		event)

	return &emptypb.Empty{}, Status(self.verbose, err)
}
