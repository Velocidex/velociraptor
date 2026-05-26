package hunt_dispatcher

import (
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

// Split a full hunt object into a summary object and a separate
// request object.
func splitHuntObject(hunt_obj *api_proto.Hunt) (
	summary_hunt_obj, request_obj *api_proto.Hunt) {

	// Only store StartRequest in this metadata object
	request_obj = &api_proto.Hunt{
		HuntId:       hunt_obj.HuntId,
		StartRequest: hunt_obj.StartRequest,
	}

	start_request := hunt_obj.StartRequest
	if start_request == nil {
		return hunt_obj, request_obj
	}

	summary_hunt_obj = proto.Clone(hunt_obj).(*api_proto.Hunt)
	summary_hunt_obj.StartRequest = &flows_proto.ArtifactCollectorArgs{
		Creator:   start_request.Creator,
		Artifacts: start_request.Artifacts,
	}

	return summary_hunt_obj, request_obj
}
