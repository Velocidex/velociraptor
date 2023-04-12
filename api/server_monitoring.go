package api

import (
	context "golang.org/x/net/context"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
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
		paths.ServerMonitoringFlowURN,
		result,
	)
	_ = err // if an error we return an empty collector args.

	return result, nil
}

func setServerMonitoringState(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string,
	args *flows_proto.ArtifactCollectorArgs) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	server_manager, err := services.GetServerEventManager(config_obj)
	if err != nil {
		return err
	}

	err = server_manager.Update(ctx, config_obj, principal, args)
	if err != nil {
		return err
	}

	return db.SetSubject(
		config_obj, paths.ServerMonitoringFlowURN,
		args)
}
