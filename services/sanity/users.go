package sanity

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
	"www.velocidex.com/golang/velociraptor/utils"
)

func createInitialUsers(
	config_obj *config_proto.Config,
	user_names []*config_proto.GUIUser) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	// We rely on the orgs to already be existing here.
	org_list := []string{"root"}
	for _, org := range config_obj.GUI.InitialOrgs {
		org_list = append(org_list, org.OrgId)
	}
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	for _, user := range user_names {
		users_manager := services.GetUserManager()
		user_record, err := users_manager.GetUser(user.Name)
		if err != nil || user_record.Name != user.Name {
			logger.Info("Initial user %v not present, creating", user.Name)
			new_user, err := users.NewUserRecord(user.Name)
			if err != nil {
				return err
			}

			// Basic auth requires setting hashed
			// password and salt
			switch strings.ToLower(config_obj.GUI.Authenticator.Type) {
			case "basic":
				new_user.PasswordHash, err = hex.DecodeString(user.PasswordHash)
				if err != nil {
					return err
				}
				new_user.PasswordSalt, err = hex.DecodeString(user.PasswordSalt)
				if err != nil {
					return err
				}

				// All other auth methods do
				// not need a password set, so
				// generate a random one
			default:
				password := make([]byte, 100)
				_, err = rand.Read(password)
				if err != nil {
					return err
				}
				users.SetPassword(new_user, string(password))
			}

			for _, org_id := range org_list {
				// Turn the org id into an org name.
				org_config_obj, err := org_manager.GetOrgConfig(org_id)
				if err != nil {
					return err
				}

				org_record := &api_proto.Org{
					Name: org_config_obj.OrgName,
					Id:   org_config_obj.OrgId,
				}

				if utils.IsRootOrg(org_id) {
					org_record.Name = "<root>"
					org_record.Id = "root"
				}
				new_user.Orgs = append(new_user.Orgs, org_record)

				// Give them the administrator role in the respective org
				err = acls.GrantRoles(
					org_config_obj, user.Name, []string{"administrator"})
				if err != nil {
					return err
				}
			}

			// Create the new user.
			err = users_manager.SetUser(new_user)
			if err != nil {
				return err
			}

			logger := logging.GetLogger(config_obj, &logging.Audit)
			logger.Info("Granting administrator role to %v because they are specified in the config's initial users",
				user.Name)
		}
	}
	return nil
}
