package sanity

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
)

func createInitialUsers(
	config_obj *config_proto.Config,
	user_names []*config_proto.GUIUser) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

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

			// Create the new user.
			err = users_manager.SetUser(new_user)
			if err != nil {
				return err
			}

			// Give them the administrator roles
			err = acls.GrantRoles(config_obj, user.Name, []string{"administrator"})
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
