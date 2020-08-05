package server

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_monitoring"
	"www.velocidex.com/golang/velociraptor/services/ddclient"
	"www.velocidex.com/golang/velociraptor/services/frontend"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/services/hunt_manager"
	"www.velocidex.com/golang/velociraptor/services/interrogation"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/sanity"
	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
	"www.velocidex.com/golang/velociraptor/services/server_monitoring"
	"www.velocidex.com/golang/velociraptor/services/vfs_service"
)

func getServerServices(config_obj *config_proto.Config) *config_proto.ServerServicesConfig {
	if config_obj.Frontend == nil {
		return &config_proto.ServerServicesConfig{}
	}

	// If no service specification is set, we start all services
	// on the primary frontend.
	if config_obj.Frontend.ServerServices == nil {
		return &config_proto.ServerServicesConfig{
			HuntManager:       true,
			HuntDispatcher:    true,
			StatsCollector:    true,
			ServerMonitoring:  true,
			ServerArtifacts:   true,
			DynDns:            true,
			Interrogation:     true,
			SanityChecker:     true,
			VfsService:        true,
			UserManager:       true,
			ClientMonitoring:  true,
			MonitoringService: true,
			ApiServer:         true,
			FrontendServer:    true,
			GuiServer:         true,
		}
	}

	return config_obj.Frontend.ServerServices
}

// Some services run on all frontends. These must all run without
// exception so they can not be selectively started.
func StartFrontendServices(config_obj *config_proto.Config,
	sm *services.Service, frontend_node string) error {

	spec := getServerServices(config_obj)

	err := sm.Start(journal.StartJournalService)
	if err != nil {
		return err
	}
	// Allow for low latency scheduling by notifying clients of
	// new events for them.
	err = sm.Start(services.StartNotificationService)
	if err != nil {
		return err
	}

	// Check everything is ok before we can start.
	err = sm.Start(sanity.StartSanityCheckService)
	if err != nil {
		return err
	}

	// Start the frontend service if needed.
	err = sm.Start(func(ctx context.Context, wg *sync.WaitGroup,
		config_obj *config_proto.Config) error {
		return frontend.StartFrontendService(ctx, config_obj, frontend_node)
	})
	if err != nil {
		return err
	}

	// Maintans the client's event monitoring table. All frontends
	// need to follow this so they can propagate changes to
	// clients.
	if spec.ClientMonitoring {
		err = sm.Start(client_monitoring.StartClientMonitoringService)
		if err != nil {
			return err
		}
	}

	// Updates DynDNS records if needed. Frontends need to maintain their IP addresses.
	if spec.DynDns {
		err = sm.Start(ddclient.StartDynDNSService)
		if err != nil {
			return err
		}
	}

	if spec.HuntDispatcher {
		// Hunt dispatcher manages client's hunt membership.
		err = sm.Start(hunt_dispatcher.StartHuntDispatcher)
		if err != nil {
			return err
		}
	}

	err = sm.Start(inventory.StartInventoryService)
	if err != nil {
		return err
	}

	err = sm.Start(launcher.StartLauncherService)
	if err != nil {
		return err
	}

	if spec.HuntManager {
		err := sm.Start(hunt_manager.StartHuntManager)
		if err != nil {
			return err
		}
	}

	// Interrogation service populates indexes etc for new
	// clients.
	if spec.Interrogation {
		err = sm.Start(interrogation.StartInterrogationService)
		if err != nil {
			return err
		}
	}

	// Runs server event queries. Should only run on one frontend.
	if spec.ServerMonitoring {
		err = sm.Start(server_monitoring.StartServerMonitoringService)
		if err != nil {
			return err
		}
	}

	// VFS service maintains the VFS GUI structures by parsing the
	// output of VFS artifacts collected.
	if spec.VfsService {
		err = sm.Start(vfs_service.StartVFSService)
		if err != nil {
			return err
		}
	}

	// Run any server artifacts the user asks for.
	if spec.ServerArtifacts {
		err = sm.Start(server_artifacts.StartServerArtifactService)
		if err != nil {
			return err
		}
	}

	return nil
}
