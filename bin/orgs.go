package main

import (
	"fmt"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/services/users"
)

var (
	// Command line interface for VQL commands.
	orgs_command = app.Command("orgs", "Manage orgs")

	orgs_ls     = orgs_command.Command("ls", "List all orgs")
	orgs_create = orgs_command.Command("create", "Create a new org")

	orgs_create_name = orgs_create.Arg("name", "Name of the new org").Required().String()

	orgs_user_add     = orgs_command.Command("user_add", "Add a user to the org")
	orgs_user_add_org = orgs_user_add.Arg("org_id", "Org ID to add user to").
				Required().String()
	orgs_user_add_user = orgs_user_add.Arg("username", "Username to add").
				Required().String()
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

func doOrgUserAdd() error {
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

	err = sm.Start(users.StartUserManager)
	if err != nil {
		return err
	}

	err = sm.Start(orgs.StartOrgManager)
	if err != nil {
		return err
	}

	user_manager := services.GetUserManager()
	record, err := user_manager.GetUserWithHashes(
		config_obj, *orgs_user_add_user)
	if err != nil {
		return err
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	org_record, err := org_manager.GetOrgConfig(*orgs_user_add_org)
	if err != nil {
		return err
	}

	record.Orgs = append(record.Orgs, &api_proto.Org{
		Name: org_record.OrgName,
		Id:   org_record.OrgId,
	})

	return user_manager.SetUser(config_obj, record)
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

		case orgs_user_add.FullCommand():
			FatalIfError(orgs_user_add, doOrgUserAdd)

		default:
			return false
		}
		return true
	})
}
