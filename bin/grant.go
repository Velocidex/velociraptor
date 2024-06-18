package main

import (
	"fmt"
	"io"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	acl_command = app.Command(
		"acl", "Manipulate acls.")

	show_command = acl_command.Command(
		"show", "Show a principal's policy.")

	show_command_principal = show_command.Arg(
		"principal", "Name of principal to grant.").
		Required().String()

	show_command_effective = show_command.Flag(
		"effective", "Show the effective persmissions object.").
		Bool()

	grant_command = acl_command.Command(
		"grant", "Grant a principal  a policy.")

	grant_command_org = grant_command.Flag("org", "OrgID to grant").String()

	grant_command_principal = grant_command.Arg(
		"principal", "Name of principal (User or cert) to grant.").
		Required().String()

	grant_command_policy_object = grant_command.Arg(
		"policy", "A policy to grant as a json encoded string").
		Default("{}").String()

	grant_command_roles = grant_command.Flag(
		"role", "A comma separated list of roles to grant the principal").
		String()

	grant_command_policy_merge = grant_command.Flag(
		"merge", "If specified we merge this policy with the old policy.").
		Bool()
)

func doGrant() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	principal := *grant_command_principal

	// Check the user actually exists first
	user_manager := services.GetUserManager()
	_, err = user_manager.GetUser(ctx,
		utils.GetSuperuserName(config_obj), principal)
	if err != nil {
		return err
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	org_config_obj, err := org_manager.GetOrgConfig(*grant_command_org)
	if err != nil {
		return err
	}

	existing_policy, err := services.GetPolicy(org_config_obj, principal)
	if err != nil && err != io.EOF {
		existing_policy = &acl_proto.ApiClientACL{}
	}

	new_policy := &acl_proto.ApiClientACL{}

	// Parse the policy object
	if *grant_command_policy_merge {
		serialized, err := json.Marshal(existing_policy)
		if err != nil {
			return fmt.Errorf("Invalid policy object: %w", err)
		}

		patched, err := jsonpatch.MergePatch(
			serialized, []byte(*grant_command_policy_object))
		if err != nil {
			return fmt.Errorf("Applying patch: %w", err)
		}

		err = json.Unmarshal(patched, &new_policy)
		if err != nil {
			return fmt.Errorf("Invalid patched policy object: %w", err)
		}
	} else {
		err = json.Unmarshal([]byte(*grant_command_policy_object),
			&new_policy)
		if err != nil {
			return fmt.Errorf("Invalid policy object: %w", err)
		}
	}

	if *grant_command_roles != "" {
		for _, role := range strings.Split(*grant_command_roles, ",") {
			if !utils.InString(new_policy.Roles, role) {
				if !acls.ValidateRole(role) {
					if err != nil {
						return fmt.Errorf("Invalid role %v", role)
					}
				}
				new_policy.Roles = append(new_policy.Roles, role)
			}
		}
	}

	err = services.SetPolicy(org_config_obj, principal, new_policy)
	if err != nil {
		return err
	}

	fmt.Println(ServerChangeWarning)
	return nil
}

func doShow() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	principal := *show_command_principal

	existing_policy, err := services.GetPolicy(config_obj, principal)
	if err != nil {
		return fmt.Errorf("Unable to load existing policy for '%v': %v ",
			principal, err)
	}

	if *show_command_effective {
		existing_policy, err = services.GetEffectivePolicy(config_obj, principal)
		if err != nil {
			return fmt.Errorf("Unable to load existing policy for '%v' ",
				principal)
		}
	}

	serialized, err := json.Marshal(existing_policy)
	if err != nil {
		return err
	}
	fmt.Println(string(serialized))
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case grant_command.FullCommand():
			FatalIfError(grant_command, doGrant)

		case show_command.FullCommand():
			FatalIfError(show_command, doShow)

		default:
			return false
		}

		return true
	})
}
