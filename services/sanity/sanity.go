package sanity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/users"
	"www.velocidex.com/golang/velociraptor/utils"
)

// This service checks the running server environment for sane
// conditions.
type SanityChecks struct{}

func (self *SanityChecks) Check(
	ctx context.Context, config_obj *config_proto.Config) error {
	if config_obj.Logging != nil &&
		config_obj.Logging.OutputDirectory != "" {
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
	if config_obj.GUI != nil && config_obj.GUI.Authenticator != nil {
		for _, user := range config_obj.GUI.InitialUsers {
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
					new_user.PasswordHash, _ = hex.DecodeString(user.PasswordHash)
					new_user.PasswordSalt, _ = hex.DecodeString(user.PasswordSalt)

					// All other auth methods do
					// not need a password set, so
					// generate a random one
				default:
					password := make([]byte, 100)
					_, _ = rand.Read(password)
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
	}

	if config_obj.Frontend != nil {
		if config_obj.Frontend.Resources.ExpectedClients == 0 {
			config_obj.Frontend.Resources.ExpectedClients = 10000
		}

		// DynDns.Hostname is deprecated, moved to Frontend.Hostname
		if config_obj.Frontend.Hostname == "" &&
			config_obj.Frontend.DynDns != nil &&
			config_obj.Frontend.DynDns.Hostname != "" {
			config_obj.Frontend.Hostname = config_obj.Frontend.DynDns.Hostname
		}
	}

	if config_obj.AutocertCertCache != "" {
		err := utils.CheckDirWritable(config_obj.AutocertCertCache)
		if err != nil {
			return errors.Wrap(
				err, fmt.Sprintf("Autocert cache directory not writable %v: ",
					config_obj.AutocertCertCache))
		}
	}

	// Reindex all the notebooks.
	notebooks, err := reporting.GetAllNotebooks(config_obj)
	if err != nil {
		return err
	}

	for _, notebook := range notebooks {
		if !strings.HasPrefix(notebook.NotebookId, "N.H.") &&
			!strings.HasPrefix(notebook.NotebookId, "N.F.") {
			err = reporting.UpdateShareIndex(config_obj, notebook)
			if err != nil {
				return err
			}
		}
	}

	err = configServerMetadata(ctx, config_obj)
	if err != nil {
		return err
	}

	return checkForServerUpgrade(ctx, config_obj)
}

// Sets the server metadata to defaults.
func configServerMetadata(
	ctx context.Context, config_obj *config_proto.Config) error {

	client_path_manager := paths.NewClientPathManager("server")
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	result := &api_proto.ClientMetadata{}
	err = db.GetSubject(config_obj, client_path_manager.Metadata(), result)
	if errors.Is(err, os.ErrNotExist) {
		// Metadata not set, start with empty set.
		err = nil
	}

	is_set := func(field string) bool {
		for _, item := range result.Items {
			if item.Key == field {
				return true
			}
		}
		return false
	}

	// Ensure the expected fields are defined
	var was_changed bool
	for _, field := range []string{"SlackToken"} {
		if !is_set(field) {
			was_changed = true
			result.Items = append(result.Items, &api_proto.ClientMetadataItem{
				Key: field,
			})
		}
	}

	if was_changed {
		err = db.SetSubject(config_obj, client_path_manager.Metadata(), result)
		if err != nil {
			return err
		}
	}

	return nil
}

// If the server is upgraded we need to do some housekeeping.
func checkForServerUpgrade(
	ctx context.Context, config_obj *config_proto.Config) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	state := &api_proto.ServerState{}
	state_path_manager := &paths.ServerStatePathManager{}

	// If the current state is not there it will have version = 0
	_ = db.GetSubject(config_obj, state_path_manager.Path(), state)
	if config_obj.Version == nil {
		return errors.New("config_obj.Version not configured")
	}

	if utils.CompareVersions(state.Version, config_obj.Version.Version) < 0 {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Server upgrade detected %v -> %v... running upgrades.",
			state.Version, config_obj.Version.Version)

		state.Version = config_obj.Version.Version
		err = db.SetSubject(config_obj, state_path_manager.Path(), state)
		if err != nil {
			return err
		}

		// Go through all the artifacts and update their tool
		// definitions.
		manager, err := services.GetRepositoryManager()
		if err != nil {
			return err
		}

		repository, err := manager.GetGlobalRepository(config_obj)
		if err != nil {
			return err
		}

		inventory := services.GetInventory()

		seen := make(map[string]bool)

		for _, name := range repository.List() {
			artifact, pres := repository.Get(config_obj, name)
			if !pres {
				continue
			}

			for _, tool_definition := range artifact.Tools {
				// This might be a manually maintained tool.
				if tool_definition.Url == "" &&
					tool_definition.GithubProject == "" {
					continue
				}

				_, pres := seen[tool_definition.Name]
				if !pres {
					seen[tool_definition.Name] = true

					// If the existing tool
					// definition was overridden
					// by the admin do not alter
					// it.
					tool, err := inventory.ProbeToolInfo(tool_definition.Name)
					if err == nil && tool.AdminOverride {
						logger.Info("<red>Skipping update</> of tool <green>%v</> because an admin manually overrode its definition.",
							tool_definition.Name)
						continue
					}

					// Log that the tool is upgraded.
					logger.WithFields(logrus.Fields{
						"Tool": tool_definition,
					}).Info("Upgrading tool <red>" + tool_definition.Name)

					// Re-add the tool to force
					// hashes to be taken when the
					// tool is used next.
					tool_definition.Hash = ""

					err = inventory.AddTool(
						config_obj, tool_definition,
						services.ToolOptions{
							Upgrade: true,
						})
					if err != nil {
						// Errors are not fatal during upgrade.
						logger.Error("Error upgrading tool: %v", err)
					}
				}
			}
		}
	}

	return nil
}

func StartSanityCheckService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	result := &SanityChecks{}
	return result.Check(ctx, config_obj)
}
