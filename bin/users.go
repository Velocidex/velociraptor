/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/users"
)

var (
	user_command  = app.Command("user", "Manage GUI users")
	user_add      = user_command.Command("add", "Add a user. If the user already exists this allows to change their password.")
	user_add_name = user_add.Arg(
		"username", "Username to add").Required().String()
	user_add_password = user_add.Arg(
		"password",
		"The password. If not specified we read from the terminal.").String()
	user_add_readonly = user_add.Flag("read_only", "Desginate this user as read only.").Bool()

	user_show      = user_command.Command("show", "Display information about a user")
	user_show_name = user_show.Arg(
		"username", "Username to show").Required().String()
)

func doAddUser() {
	config_obj, err := get_server_config(*config_path)
	kingpin.FatalIfError(err, "Unable to load config file")

	user_record, err := users.NewUserRecord(*user_add_name)
	if err != nil {
		kingpin.FatalIfError(err, "add user:")
	}
	user_record.ReadOnly = *user_add_readonly

	if config_obj.GUI.GoogleOauthClientId != "" {
		fmt.Printf("Authentication will occur via Google - " +
			"therefore no password needs to be set.")

		password := make([]byte, 100)
		_, err = rand.Read(password)
		kingpin.FatalIfError(err, "rand")
		password_str := string(password)
		user_add_password = &password_str

	} else if *user_add_password == "" {
		fmt.Printf("Enter user's password: ")
		password, err := terminal.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			kingpin.FatalIfError(
				err, "Unable to read password from terminal.")
		}
		password_str := string(password)
		user_add_password = &password_str
	}

	user_record.SetPassword(*user_add_password)
	err = users.SetUser(config_obj, user_record)
	if err != nil {
		kingpin.FatalIfError(
			err, "Unable to set user account.")
	}
	fmt.Printf("\r\n")
}

func doShowUser() {
	config_obj, err := get_server_config(*config_path)
	kingpin.FatalIfError(err, "Unable to load config file")

	user_record, err := users.GetUser(config_obj, *user_show_name)
	kingpin.FatalIfError(err, "Unable to find user %s", *user_show_name)

	s, err := json.MarshalIndent(user_record, "", " ")
	if err == nil {
		os.Stdout.Write(s)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "user add":
			doAddUser()

		case user_show.FullCommand():
			doShowUser()

		default:
			return false
		}
		return true
	})
}
