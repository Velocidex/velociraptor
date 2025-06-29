package orgs

import (
	"context"
	"errors"
	"fmt"
	"os"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func RemoveOrgFromUsers(
	ctx context.Context, principal, org_id string) error {

	// Remove the org from all the users.
	user_manager := services.GetUserManager()
	users, err := user_manager.ListUsers(ctx, principal, services.LIST_ALL_ORGS)
	if err != nil {
		return err
	}

	for _, u := range users {
		record, err := user_manager.GetUserWithHashes(ctx, principal, u.Name)
		if err == nil {
			new_orgs := []*api_proto.OrgRecord{}
			for _, org := range record.Orgs {
				if org.Id != org_id {
					new_orgs = append(new_orgs, org)
				}
			}
			if len(new_orgs) != len(record.Orgs) {
				record.Orgs = new_orgs
				_ = user_manager.SetUser(ctx, record)
			}
		}
	}

	return nil
}

func (self *OrgManager) DeleteOrg(ctx context.Context, principal, org_id string) error {
	if utils.IsRootOrg(org_id) {
		return errors.New("Can not remove root org.")
	}

	err := RemoveOrgFromUsers(ctx, principal, org_id)
	if err != nil {
		return err
	}

	org_path_manager := paths.NewOrgPathManager(org_id)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	err = db.DeleteSubject(self.config_obj, org_path_manager.Path())
	if err != nil {
		return err
	}

	// Remove the org from the manager and cancel all its services.
	self.mu.Lock()
	org_context, pres := self.orgs[org_id]
	self.mu.Unlock()

	if !pres {
		return fmt.Errorf("Org %v does not exist.", org_id)
	}

	// Shut down the org's services
	org_context.sm.Close()

	self.mu.Lock()
	delete(self.orgs, org_id)
	delete(self.org_id_by_nonce, org_id)
	self.mu.Unlock()

	// Wait a bit for the services to shut down so we can remove files
	// safely.
	go func() {
		_ = datastore.Walk(self.config_obj, db, org_path_manager.Path(),
			datastore.WalkWithoutDirectories,
			func(path api.DSPathSpec) error {
				_ = db.DeleteSubject(self.config_obj, path)
				return nil
			})

		file_store_factory := file_store.GetFileStore(self.config_obj)
		_ = api.Walk(file_store_factory,
			org_path_manager.Path().AsFilestorePath(),
			func(path api.FSPathSpec, info os.FileInfo) error {
				// Ignore errors in deletion to attempt to delete all the files.
				_ = file_store_factory.Delete(path)
				return nil
			})
	}()

	return nil
}
