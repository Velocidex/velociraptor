package services

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/users"
)

// This service checks the running server environment for sane
// conditions.
type SanityChecks struct{}

func (self *SanityChecks) Check(config_obj *config_proto.Config) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	// If we are handling the authentication make sure there is at
	// least one user account created.
	if !config.GoogleAuthEnabled(config_obj) &&
		!config.SAMLEnabled(config_obj) {

		// Make sure all the users specified in the config
		// file exist.
		for _, user := range config_obj.GUI.InitialUsers {
			user_record, err := users.GetUser(config_obj, user.Name)
			if err != nil || user_record.Name != user.Name {
				logger.Info("Initial user %v not present, creating",
					user.Name)
				new_user, _ := users.NewUserRecord(user.Name)
				new_user.PasswordHash, _ = hex.DecodeString(user.PasswordHash)
				new_user.PasswordSalt, _ = hex.DecodeString(user.PasswordSalt)
				err := users.SetUser(config_obj, new_user)
				if err != nil {
					return err
				}
			}
		}
	}

	if config_obj.Frontend.PublicPath == "" {
		return errors.New("Frontend is missing a public_path setting. This is required to serve third party VQL plugins.")
	}

	// Ensure there is an index.html file in there to prevent directory listing.
	err := os.MkdirAll(config_obj.Frontend.PublicPath, 0700)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(filepath.Join(
		config_obj.Frontend.PublicPath,
		"index.html"), os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		return err
	}
	file.Close()

	return nil
}

func startSanityCheckService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	result := &SanityChecks{}
	return result.Check(config_obj)
}
