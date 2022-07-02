// A utility to start up all essential services.

package startup

import (
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/ddclient"
	"www.velocidex.com/golang/velociraptor/services/orgs"
)

func StartupEssentialServices(sm *services.Service) error {
	err := sm.Start(datastore.StartRemoteDatastore)
	if err != nil {
		return err
	}

	// Updates DynDNS records if needed. Frontends need to maintain
	// their IP addresses.
	err = sm.Start(ddclient.StartDynDNSService)
	if err != nil {
		return err
	}

	_, err = services.GetOrgManager()
	if err != nil {
		err = sm.Start(orgs.StartOrgManager)
		if err != nil {
			return err
		}
	}

	return nil
}

// Start usual services that run on frontends only (i.e. not the client).
func StartupFrontendServices(sm *services.Service) (err error) {
	err = sm.Start(datastore.StartMemcacheFileService)
	if err != nil {
		return err
	}

	return nil
}
