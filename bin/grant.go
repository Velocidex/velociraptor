package main

import (
	"encoding/json"
	"fmt"
	"io"

	jsonpatch "github.com/evanphx/json-patch"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/config"
)

var (
	acl_command = app.Command(
		"acl", "Manipulate api_client acls.")

	show_command = acl_command.Command(
		"show", "Grant API client a policy.")

	show_command_principal = show_command.Arg(
		"principal", "Name of certificate to grant.").
		Required().String()

	grant_command = acl_command.Command(
		"grant", "Grant API client a policy.")

	grant_command_principal = grant_command.Arg(
		"principal", "Name of certificate to grant.").
		Required().String()

	grant_command_policy_object = grant_command.Arg(
		"policy", "A policy to grant as a json encoded string").
		String()

	grant_command_policy_merge = grant_command.Flag(
		"merge", "If specified we merge this policy with the old policy.").
		Bool()
)

func doGrant() {
	config_obj, err := config.LoadConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config.")

	principal := *grant_command_principal

	existing_policy, err := acls.GetPolicy(config_obj, principal)
	if err != nil && err != io.EOF {
		kingpin.FatalIfError(err, "Unable to load existing policy for %v",
			principal)
	}

	new_policy := &acl_proto.ApiClientACL{}

	// Parse the policy object
	if *grant_command_policy_merge {
		serialized, err := json.Marshal(existing_policy)
		kingpin.FatalIfError(err, "Invalid policy object")

		patched, err := jsonpatch.MergePatch(
			serialized, []byte(*grant_command_policy_object))
		kingpin.FatalIfError(err, "Applying patch")

		err = json.Unmarshal(patched, &new_policy)
		kingpin.FatalIfError(err, "Invalid patched policy object")
	} else {
		err = json.Unmarshal([]byte(*grant_command_policy_object),
			&new_policy)
		kingpin.FatalIfError(err, "Invalid policy object")
	}

	err = acls.SetPolicy(config_obj, principal, new_policy)
	kingpin.FatalIfError(err, "Setting policy object")
}

func doShow() {
	config_obj, err := config.LoadConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config.")

	principal := *show_command_principal
	existing_policy, err := acls.GetPolicy(config_obj, principal)
	kingpin.FatalIfError(err, "Unable to load existing policy for '%v' ",
		principal)

	serialized, err := json.Marshal(existing_policy)
	kingpin.FatalIfError(err, "Unable to serialized policy ")
	fmt.Println(string(serialized))
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case grant_command.FullCommand():
			doGrant()

		case show_command.FullCommand():
			doShow()

		default:
			return false
		}

		return true
	})
}
