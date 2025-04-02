package sanity

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

// Creates the initial orgs as specified in the GUI.initial_orgs
// list. Users can specify the org id if they wish.
func createInitialOrgs(config_obj *config_proto.Config) error {
	if config_obj.GUI == nil || config_obj.GUI.Authenticator == nil {
		return nil
	}

	// Create initial orgs
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	for _, org := range config_obj.GUI.InitialOrgs {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Creating initial org for</> %v", org.Name)
		org_record, err := org_manager.CreateNewOrg(org.Name, org.OrgId, org.Nonce)
		if err != nil {
			return err
		}

		// Receive the newly created orgid and update the config file
		// with it
		org.OrgId = org_record.Id
	}

	return nil
}
