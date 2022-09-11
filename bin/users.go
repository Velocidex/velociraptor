/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
	"www.velocidex.com/golang/velociraptor/startup"
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

	user_lock = user_command.Command(
		"lock", "Lock a user immediately by locking their account.")
	user_lock_name = user_lock.Arg(
		"username", "Username to lock").Required().String()
)

func doAddUser() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	if config_obj.Frontend == nil {
		config_obj.Frontend = &config_proto.FrontendConfig{}
	}
	config_obj.Frontend.ServerServices = services.GenericToolServices()

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

	user_record, err := users.NewUserRecord(*user_add_name)
	if err != nil {
		return fmt.Errorf("add user: %s", err)
	}

	err = acls.GrantRoles(config_obj, *user_add_name,
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
	fmt.Printf("\r\n")
	return nil
}

func doShowUser() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	if config_obj.Frontend == nil {
		config_obj.Frontend = &config_proto.FrontendConfig{}
	}
	config_obj.Frontend.ServerServices = services.GenericToolServices()

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	users_manager := services.GetUserManager()
	user_record, err := users_manager.GetUser(ctx, *user_show_name)
	if err != nil {
		return err
	}

	if *user_show_hashes {
		user_record, err := users_manager.GetUserWithHashes(ctx, *user_show_name)
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

func doLockUser() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	if config_obj.Frontend == nil {
		config_obj.Frontend = &config_proto.FrontendConfig{}
	}
	config_obj.Frontend.ServerServices = services.GenericToolServices()

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	users_manager := services.GetUserManager()
	user_record, err := users_manager.GetUser(ctx, *user_lock_name)
	if err != nil {
		return fmt.Errorf("Unable to find user %s", *user_lock_name)
	}

	user_record.Locked = true

	err = users_manager.SetUser(ctx, user_record)
	if err != nil {
		return fmt.Errorf("Unable to set user account: %w", err)
	}
	fmt.Printf("\r\n")
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case user_add.FullCommand():
			FatalIfError(user_add, doAddUser)

		case user_lock.FullCommand():
			FatalIfError(user_lock, doLockUser)

		case user_show.FullCommand():
			FatalIfError(user_show, doShowUser)

		default:
			return false
		}
		return true
	})
}
