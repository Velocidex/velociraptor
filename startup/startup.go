// A utility to start up all essential services.

package startup

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_monitoring"
	"www.velocidex.com/golang/velociraptor/services/ddclient"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/services/hunt_manager"
	"www.velocidex.com/golang/velociraptor/services/interrogation"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/services/sanity"
	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
	"www.velocidex.com/golang/velociraptor/services/server_monitoring"
	"www.velocidex.com/golang/velociraptor/services/users"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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
			IndexServer:       true,
		}
	}

	return config_obj.Frontend.ServerServices
}

func StartupEssentialServices(sm *services.Service) error {
	spec := getServerServices(sm.Config)

	err := sm.Start(datastore.StartRemoteDatastore)
	if err != nil {
		return err
	}

	// Updates DynDNS records if needed. Frontends need to maintain
	// their IP addresses.
	if spec.DynDns {
		err := sm.Start(ddclient.StartDynDNSService)
		if err != nil {
			return err
		}
	}

	_, err = services.GetOrgManager()
	if err != nil {
		err = sm.Start(orgs.StartOrgManager)
		if err != nil {
			return err
		}
	}

	launcher_obj, _ := services.GetLauncher()
	if launcher_obj == nil {
		err := sm.Start(launcher.StartLauncherService)
		if err != nil {
			return err
		}
	}

	return nil
}

// Start usual services that run on frontends only (i.e. not the client).
func StartupFrontendServices(sm *services.Service) (err error) {
	spec := getServerServices(sm.Config)

	_, err = services.GetOrgManager()
	if err != nil {
		err = sm.Start(orgs.StartOrgManager)
		if err != nil {
			return err
		}
	}

	err = sm.Start(datastore.StartMemcacheFileService)
	if err != nil {
		return err
	}

	err = sm.Start(users.StartUserManager)
	if err != nil {
		return err
	}

	// Check everything is ok before we can start.
	if spec.ClientMonitoring {
		// Maintans the client's event monitoring table. All frontends
		// need to follow this so they can propagate changes to
		// clients.
		err := sm.Start(client_monitoring.StartClientMonitoringService)
		if err != nil {
			return err
		}
	}

	err = sm.Start(notebook.StartNotebookManagerService)
	if err != nil {
		return err
	}

	if spec.HuntDispatcher {
		// Hunt dispatcher manages client's hunt membership.
		err := sm.Start(hunt_dispatcher.StartHuntDispatcher)
		if err != nil {
			return err
		}
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
		err := sm.Start(interrogation.StartInterrogationService)
		if err != nil {
			return err
		}
	}

	// Runs server event queries. Should only run on one frontend.
	if spec.ServerMonitoring {
		err := sm.Start(server_monitoring.StartServerMonitoringService)
		if err != nil {
			return err
		}
	}

	// Run any server artifacts the user asks for.
	if spec.ServerArtifacts {
		err := sm.Start(server_artifacts.StartServerArtifactService)
		if err != nil {
			return err
		}
	}

	// Sanity checker needs to start last so it can check all the
	// other services.
	if spec.SanityChecker {
		err := sm.Start(sanity.StartSanityCheckService)
		if err != nil {
			return err
		}
	}

	return nil
}

func Reset(config_obj *config_proto.Config) {
	// This function should not find any active services. Services
	// are responsible for unregistering themselves and holding
	// the service manager for the duration of their lifetime.

	journal, _ := services.GetJournal(config_obj)
	if journal != nil {
		fmt.Printf("Journal not reset.\n")
	}

	if services.GetNotifier() != nil {
		fmt.Printf("Notifier not reset.\n")
	}

	_, err := services.GetInventory(config_obj)
	if err != nil {
		fmt.Printf("Inventory not reset.\n")
	}

	launcher, _ := services.GetLauncher()
	if launcher != nil {
		fmt.Printf("Launcher not reset.\n")
	}

	if services.GetHuntDispatcher() != nil {
		fmt.Printf("HuntDispatcher not reset.\n")
	}
}
