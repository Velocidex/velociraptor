package services

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/users"
)

// This service checks the running server environment for sane
// conditions.
type SanityChecks struct{}

func (self *SanityChecks) Close() {}

func (self *SanityChecks) Check(config_obj *config_proto.Config) error {
	// If we are handling the authentication make sure there is at
	// least one user account created.
	if !config.GoogleAuthEnabled(config_obj) &&
		!config.SAMLEnabled(config_obj) {
		user_records, err := users.ListUsers(config_obj)
		if err != nil {
			return err
		}

		if len(user_records) == 0 {
			return errors.New("Local authentication configured, but there are no user accounts defined. You need to make at least one admin user by running 'velociraptor --config server.config.yaml user add <username>'")
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

func startSanityCheckService(config_obj *config_proto.Config) (
	*SanityChecks, error) {
	result := &SanityChecks{}

	return result, result.Check(config_obj)
}
