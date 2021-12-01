package sanity

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/users"
)

func createInitialUsers(
	config_obj *config_proto.Config,
	user_names []*config_proto.GUIUser) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	for _, user := range user_names {
		user_record, err := users.GetUser(config_obj, user.Name)
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
			err = users.SetUser(config_obj, new_user)
			if err != nil {
				return err
			}

			// Give them the administrator roles
			err = acls.GrantRoles(config_obj, user.Name, []string{"administrator"})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
