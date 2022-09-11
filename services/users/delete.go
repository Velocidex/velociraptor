package users

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *UserManager) DeleteUser(
	ctx context.Context,
	org_config_obj *config_proto.Config, username string) error {

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	// Get the root org config because users are managed in the root
	// org.
	root_config_obj, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return err
	}

	db, err := datastore.GetDB(root_config_obj)
	if err != nil {
		return err
	}

	user_path_manager := paths.NewUserPathManager(username)
	err = db.DeleteSubject(root_config_obj, user_path_manager.Path())
	if err != nil {
		return err
	}

	// Also remove the ACLs for the user from all orgs.
	for _, org_record := range org_manager.ListOrgs() {
		org_config_obj, err := org_manager.GetOrgConfig(org_record.OrgId)
		if err != nil {
			continue
		}

		err = db.DeleteSubject(org_config_obj, user_path_manager.ACL())
		if err != nil {
			continue
		}
	}

	return nil
}
