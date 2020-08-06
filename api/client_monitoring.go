package api

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func getClientMonitoringState(config_obj *config_proto.Config, label string) (
	*api_proto.GetMonitoringStateResponse, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	table := &flows_proto.ClientEventTable{}
	err = db.GetSubject(config_obj,
		constants.ClientMonitoringFlowURN,
		table,
	)
	_ = err // if an error we return an empty collector args.
	return &api_proto.GetMonitoringStateResponse{
		Requests: []*api_proto.SetMonitoringStateRequest{
			{Request: table.Artifacts},
		},
	}, nil
}

func setClientMonitoringState(
	config_obj *config_proto.Config,
	args *api_proto.SetMonitoringStateRequest) error {
	return services.ClientEventManager().UpdateClientEventTable(
		config_obj, args.Request, args.Label)
}
