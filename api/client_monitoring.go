package api

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func getClientMonitoringState(config_obj *api_proto.Config) (
	*flows_proto.ArtifactCollectorArgs, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	result := &flows_proto.ClientEventTable{}
	err = db.GetSubject(config_obj,
		constants.ClientMonitoringFlowURN,
		result,
	)
	_ = err // if an error we return an empty collector args.
	return result.Artifacts, nil
}

func setClientMonitoringState(
	config_obj *api_proto.Config,
	args *flows_proto.ArtifactCollectorArgs) error {
	return services.UpdateClientEventTable(config_obj, args)
}
