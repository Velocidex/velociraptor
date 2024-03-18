package sanity

import (
	"context"
	"errors"
	"os"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

// Check if this is the first ever run.
func isFirstRun(ctx context.Context,
	config_obj *config_proto.Config) bool {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return false
	}

	path_manager := paths.ServerStatePathManager{}
	install_record := &api_proto.ServerInstallRecord{}
	err = db.GetSubject(config_obj, path_manager.Install(), install_record)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return true
	}

	if err != nil {
		return false
	}

	return install_record.InstallTime == 0
}

// Sets the install time record so we know never to run initial things
// again.
func setFirstRun(ctx context.Context,
	config_obj *config_proto.Config) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	path_manager := paths.ServerStatePathManager{}
	install_record := &api_proto.ServerInstallRecord{
		InstallTime: uint64(time.Now().Unix()),
	}

	return db.SetSubject(config_obj, path_manager.Install(), install_record)
}

// Start the initial artifacts specified in the config file. Should
// only happen on first install run.
func startInitialArtifacts(
	ctx context.Context, config_obj *config_proto.Config) error {

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
		principal := utils.GetSuperuserName(config_obj)
		_, err = launcher.ScheduleArtifactCollection(ctx, config_obj,
			acl_managers.NewRoleACLManager(config_obj, "administrator"),
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
