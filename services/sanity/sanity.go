package sanity

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"

	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// This service checks the running server environment for sane
// conditions.
type SanityChecks struct{}

// Check sanity of general server state - this is only done for the root org.
func (self *SanityChecks) CheckRootOrg(
	ctx context.Context, config_obj *config_proto.Config) error {
	if config_obj.Logging != nil &&
		config_obj.Logging.OutputDirectory != "" {
		err := utils.CheckDirWritable(config_obj.Logging.OutputDirectory)
		if err != nil {
			return fmt.Errorf("Unable to write logs to directory %v: %w",
				config_obj.Logging.OutputDirectory, err)
		}
	}

	if isFirstRun(ctx, config_obj) {
		// Create any initial orgs required.
		err := createInitialOrgs(config_obj)
		if err != nil {
			return err
		}

		// Make sure the initial user accounts are created with the
		// administrator roles.
		err = createInitialUsers(ctx, config_obj)
		if err != nil {
			return err
		}

		err = startInitialArtifacts(ctx, config_obj)
		if err != nil {
			return err
		}

		err = setFirstRun(ctx, config_obj)
		if err != nil {
			return err
		}
	}

	err := self.CheckForLockdown(ctx, config_obj)
	if err != nil {
		return err
	}

	err = self.CheckFrontendSettings(config_obj)
	if err != nil {
		return err
	}

	err = self.CheckDatastoreSettings(config_obj)
	if err != nil {
		return err
	}

	err = self.CheckSecuritySettings(config_obj)
	if err != nil {
		return err
	}

	err = self.CheckAPISettings(config_obj)
	if err != nil {
		return err
	}

	// Make sure our internal VelociraptorServer service account is
	// properly created. Default accounts are created with org admin
	// so they can add new orgs as required.
	service_account_name := utils.GetSuperuserName(config_obj)
	err = services.GrantRoles(
		config_obj, service_account_name,
		[]string{"administrator", "org_admin"})
	if err != nil {
		return err
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

		if config_obj.Frontend.CollectionErrorRegex != "" {
			_, err := regexp.Compile(config_obj.Frontend.CollectionErrorRegex)
			if err != nil {
				return fmt.Errorf(
					"Frontend.collection_error_regex is invalid: %w", err)
			}
		}
	}

	if config_obj.AutocertCertCache != "" {
		err := utils.CheckDirWritable(config_obj.AutocertCertCache)
		if err != nil {
			return fmt.Errorf("Autocert cache directory not writable %v: %w",
				config_obj.AutocertCertCache, err)
		}
	}

	return nil
}

func (self *SanityChecks) Check(
	ctx context.Context, config_obj *config_proto.Config) error {

	if utils.IsRootOrg(config_obj.OrgId) {
		err := self.CheckRootOrg(ctx, config_obj)
		if err != nil {
			return err
		}
	}

	err := configServerMetadata(ctx, config_obj)
	if err != nil {
		return err
	}

	err = maybeMigrateClientIndex(ctx, config_obj)
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
	_ = db.GetSubject(config_obj, client_path_manager.Metadata(), result)

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

	if utils.CompareVersions("velociraptor",
		state.Version, config_obj.Version.Version) < 0 {
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
		manager, err := services.GetRepositoryManager(config_obj)
		if err != nil {
			return err
		}

		repository, err := manager.GetGlobalRepository(config_obj)
		if err != nil {
			return err
		}

		inventory, err := services.GetInventory(config_obj)
		if err != nil {
			return err
		}

		seen := make(map[string]bool)

		names, err := repository.List(ctx, config_obj)
		if err != nil {
			return err
		}

		for _, name := range names {
			artifact, pres := repository.Get(ctx, config_obj, name)
			if !pres {
				continue
			}

			for _, tool_definition := range artifact.Tools {
				// This might be a manually maintained tool.
				if tool_definition.Url == "" &&
					tool_definition.GithubProject == "" {
					continue
				}

				key := tool_definition.Name
				if tool_definition.Version != "" {
					key = tool_definition.Name + ":" + tool_definition.Version
				}
				_, pres := seen[key]
				if !pres {
					seen[key] = true

					// If the existing tool definition was overridden
					// by the admin do not alter it.
					tool, err := inventory.ProbeToolInfo(
						ctx, config_obj, tool_definition.Name, tool_definition.Version)
					if err == nil && tool.AdminOverride {
						logger.Info("<yellow>Skipping update</> of tool <green>%v</> because an admin manually overrode its definition.",
							tool_definition.Name)
						continue
					}

					// Log that the tool is upgraded.
					logger.WithFields(logrus.Fields{
						"Tool": tool_definition,
					}).Info("Upgrading tool <green>" + key)

					tool_definition = proto.Clone(
						tool_definition).(*artifacts_proto.Tool)

					// Re-add the tool to force hashes to be taken
					// when the tool is used next.
					tool_definition.Hash = ""

					err = inventory.AddTool(ctx,
						config_obj, tool_definition,
						services.ToolOptions{
							Upgrade:            true,
							ArtifactDefinition: true,
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

func NewSanityCheckService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	result := &SanityChecks{}
	return result.Check(ctx, config_obj)
}
