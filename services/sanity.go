package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/users"
	"www.velocidex.com/golang/velociraptor/utils"
)

// This service checks the running server environment for sane
// conditions.
type SanityChecks struct{}

func (self *SanityChecks) Check(config_obj *config_proto.Config) error {
	if config_obj.Logging.OutputDirectory != "" {
		err := utils.CheckDirWritable(config_obj.Logging.OutputDirectory)
		if err != nil {
			return errors.Wrap(
				err, fmt.Sprintf("Unable to write logs to directory %v: ",
					config_obj.Logging.OutputDirectory))
		}
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	// Make sure the initial user accounts are created with the
	// administrator roles.
	for _, user := range config_obj.GUI.InitialUsers {
		user_record, err := users.GetUser(config_obj, user.Name)
		if err != nil || user_record.Name != user.Name {
			logger.Info("Initial user %v not present, creating",
				user.Name)
			new_user, _ := users.NewUserRecord(user.Name)

			if config.GoogleAuthEnabled(config_obj) ||
				config.SAMLEnabled(config_obj) {
				password := make([]byte, 100)
				rand.Read(password)
				new_user.SetPassword(string(password))

			} else {
				new_user.PasswordHash, _ = hex.DecodeString(user.PasswordHash)
				new_user.PasswordSalt, _ = hex.DecodeString(user.PasswordSalt)
			}
			err := users.SetUser(config_obj, new_user)
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

	if config_obj.Frontend.ExpectedClients == 0 {
		config_obj.Frontend.ExpectedClients = 10000
	}

	// DynDns.Hostname is deprecated, moved to Frontend.Hostname
	if config_obj.Frontend.Hostname == "" && config_obj.Frontend.Hostname != "" {
		config_obj.Frontend.Hostname = config_obj.Frontend.Hostname
	}

	// Ensure there is an index.html file in there to prevent directory listing.
	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.WriteFile("/public/index.html")
	if err != nil {
		return err
	}
	fd.Close()

	if config_obj.AutocertCertCache != "" {
		err := utils.CheckDirWritable(config_obj.AutocertCertCache)
		if err != nil {
			return errors.Wrap(
				err, fmt.Sprintf("Autocert cache directory not writable %v: ",
					config_obj.AutocertCertCache))
		}
	}

	return nil
}

func startSanityCheckService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	result := &SanityChecks{}
	return result.Check(config_obj)
}
