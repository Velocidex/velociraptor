package main

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
)

var (
	// Command line interface for VQL commands.
	orgs_command = app.Command("orgs", "Manage orgs")

	orgs_ls     = orgs_command.Command("ls", "List all orgs")
	orgs_create = orgs_command.Command("create", "Create a new org")

	orgs_create_name = orgs_create.Arg("name", "Name of the new org").Required().String()
)

func doOrgLs() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().
		WithRequiredLogging().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	err = sm.Start(orgs.StartOrgManager)
	if err != nil {
		return err
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	for _, org := range org_manager.ListOrgs() {
		fmt.Println(string(json.MustMarshalIndent(org)))
	}

	return nil
}

func doOrgCreate() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().
		WithRequiredLogging().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	err = sm.Start(orgs.StartOrgManager)
	if err != nil {
		return err
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	record, err := org_manager.CreateNewOrg(*orgs_create_name)
	if err != nil {
		return err
	}

	fmt.Println(string(json.MustMarshalIndent(record)))

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case orgs_ls.FullCommand():
			FatalIfError(orgs_ls, doOrgLs)

		case orgs_create.FullCommand():
			FatalIfError(orgs_create, doOrgCreate)

		default:
			return false
		}
		return true
	})
}
