/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh/terminal"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	ServerChangeWarning = `
NOTE: This command changes the underlying data in the data store.

These changes may not be immediately visible to a running server
so you should restart the server to pick up these changes.
The recommended way to make these changes is via the API.
See the following for more information

https://docs.velociraptor.app/docs/server_automation/server_api/`
)

var (
	user_command  = app.Command("user", "Manage GUI users")
	user_add      = user_command.Command("add", "Add a user. If the user already exists this allows to change their password.")
	user_add_name = user_add.Arg(
		"username", "Username to add").Required().String()
	user_add_password = user_add.Arg(
		"password",
		"The password. If not specified we read from the terminal.").String()
	user_add_roles = user_add.Flag("role", "Specify the role for this user.").
			Required().String()

	user_show      = user_command.Command("show", "Display information about a user")
	user_show_name = user_show.Arg(
		"username", "Username to show").Required().String()
	user_show_hashes = user_show.Flag("with_hashes", "Displays the password hashes too.").
				Bool()
)

func doAddUser() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	config_obj.Services = services.GenericToolServices()

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	err = sm.Start(users.StartUserManager)
	if err != nil {
		return err
	}

	user_record, err := users.NewUserRecord(config_obj, *user_add_name)
	if err != nil {
		return fmt.Errorf("add user: %s", err)
	}

	err = services.GrantRoles(config_obj, *user_add_name,
		strings.Split(*user_add_roles, ","))
	if err != nil {
		return err
	}

	authenticator, err := authenticators.NewAuthenticator(config_obj)
	if err != nil {
		return fmt.Errorf("Granting roles: %w", err)
	}

	if authenticator.IsPasswordLess() {
		fmt.Printf("Authentication will occur via %v - "+
			"therefore no password needs to be set.",
			config_obj.GUI.Authenticator.Type)

		password := make([]byte, 100)
		_, err = rand.Read(password)
		if err != nil {
			return err
		}
		password_str := string(password)
		user_add_password = &password_str

	} else if *user_add_password == "" {
		fmt.Printf("Enter user's password: ")
		password, err := terminal.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("Unable to read password from terminal: %w", err)
		}
		password_str := string(password)
		user_add_password = &password_str
	}

	users.SetPassword(user_record, *user_add_password)

	users_manager := services.GetUserManager()
	err = users_manager.SetUser(ctx, user_record)
	if err != nil {
		return fmt.Errorf("Unable to set user account: %w", err)
	}
	fmt.Println(ServerChangeWarning)
	return nil
}

func doShowUser() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	config_obj.Services = services.GenericToolServices()

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	users_manager := services.GetUserManager()
	user_record, err := users_manager.GetUser(ctx,
		utils.GetSuperuserName(config_obj), *user_show_name)
	if err != nil {
		return err
	}

	if *user_show_hashes {
		user_record, err := users_manager.GetUserWithHashes(ctx,
			utils.GetSuperuserName(config_obj), *user_show_name)
		if err != nil {
			return err
		}

		fmt.Println("The following are suitable to add into the initial users field of the config file.")
		fmt.Printf("Password hash is %02x\n", user_record.PasswordHash)
		fmt.Printf("Password salt is %02x\n", user_record.PasswordSalt)
	}

	s, err := json.MarshalIndent(user_record)
	if err != nil {
		return err
	}
	os.Stdout.Write(s)
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case user_add.FullCommand():
			FatalIfError(user_add, doAddUser)

		case user_show.FullCommand():
			FatalIfError(user_show, doShowUser)

		default:
			return false
		}
		return true
	})
}
