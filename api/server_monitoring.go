package api

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func getServerMonitoringState(config_obj *config_proto.Config) (
	*flows_proto.ArtifactCollectorArgs, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	result := &flows_proto.ArtifactCollectorArgs{}
	err = db.GetSubject(config_obj,
		constants.ServerMonitoringFlowURN,
		result,
	)
	_ = err // if an error we return an empty collector args.
	return result, nil
}

func setServerMonitoringState(
	config_obj *config_proto.Config,
	args *flows_proto.ArtifactCollectorArgs) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = services.GetServerEventManager().Update(config_obj, args)
	if err != nil {
		return err
	}

	return db.SetSubject(
		config_obj, constants.ServerMonitoringFlowURN,
		args)
}
