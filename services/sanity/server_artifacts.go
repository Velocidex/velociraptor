package sanity

import (
	"context"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func maybeStartInitialArtifacts(
	ctx context.Context, config_obj *config_proto.Config) error {

	path_manager := paths.ServerStatePathManager{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	install_record := &api_proto.ServerInstallRecord{}
	err = db.GetSubject(config_obj, path_manager.Install(), install_record)
	if err == nil && install_record.InstallTime > 0 {
		return nil
	}

	// Install record does not exist! make a new one
	install_record.InstallTime = uint64(time.Now().Unix())

	err = db.SetSubject(config_obj, path_manager.Install(), install_record)
	if err != nil {
		return err
	}
	// Start any initial artifact collections.
	if config_obj.Frontend != nil &&
		len(config_obj.Frontend.InitialServerArtifacts) > 0 {

		manager, err := services.GetRepositoryManager(config_obj)
		if err != nil {
			return err
		}

		repository, err := manager.GetGlobalRepository(config_obj)
		if err != nil {
			return err
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			return err
		}

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

		// Run artifacts with full privileges.
		principal := ""
		if config_obj.Client != nil {
			principal = config_obj.Client.PinnedServerName
		}

		_, err = launcher.ScheduleArtifactCollection(ctx, config_obj,
			vql_subsystem.NewRoleACLManager("administrator"),
			repository,
			&flows_proto.ArtifactCollectorArgs{
				Creator:   principal,
				ClientId:  "server",
				Artifacts: config_obj.Frontend.InitialServerArtifacts,
			}, nil)
		if err != nil {
			logger.Error("Launching initial artifacts: %v", err)
			return err
		}

		logger.Info("Launched initial artifacts: <green>%v</>",
			config_obj.Frontend.InitialServerArtifacts)
	}
	return nil
}
