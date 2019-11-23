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
// Inspect the file store and decode the stored objects.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/olekukonko/tablewriter"
	errors "github.com/pkg/errors"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var classifiers = map[string]proto.Message{
	"/clients/C.[^/]+$":                       &actions_proto.ClientInfo{},
	"/clients/C.[^/]+/ping$":                  &actions_proto.ClientInfo{},
	"/clients/C.[^/]+/key$":                   &crypto_proto.PublicKey{},
	"/clients/C.[^/]+/vfs/.+":                 &actions_proto.VQLResponse{},
	"/clients/C.[^/]+/collections/F\\.[^/]+$": &flows_proto.ArtifactCollectorContext{},
	"/clients/C.[^/]+/tasks/[^/]+$":           &crypto_proto.GrrMessage{},
	constants.HUNTS_URN + "H.[^/]+$":          &api_proto.Hunt{},
	"/users/[^/]+$":                           &api_proto.VelociraptorUser{},
	"/users/[^/]+/notifications/.+$":          &api_proto.UserNotification{},
}

var (
	// Inspect the filestore
	inspect_command = app.Command(
		"inspect", "Inspect datastore files.")
	inspect_filename = inspect_command.Arg(
		"filename", "The filename from the filestore").
		Required().String()
)

func getProto(urn string) (proto.Message, error) {
	for k, v := range classifiers {
		m, err := regexp.MatchString(k, urn)
		if m && err == nil {
			return v, nil
		}
	}

	return nil, errors.New(fmt.Sprintf(
		"Unknown URN pattern: %v", urn))
}

func renderTable(response *actions_proto.VQLResponse) error {
	scope := vfilter.NewScope()
	table := tablewriter.NewWriter(os.Stdout)
	defer table.Render()

	table.SetHeader(response.Columns)
	table.SetCaption(true, response.Query.Name+": "+response.Query.VQL)

	data := []map[string]interface{}{}
	err := json.Unmarshal([]byte(response.Response), &data)
	if err != nil {
		return err
	}
	for _, row := range data {
		string_row := []string{}
		for _, k := range response.Columns {
			item, pres := row[k]
			if !pres {
				item = ""
			}
			string_row = append(string_row, utils.Stringify(
				item, scope, 120/len(response.Columns)))
		}

		table.Append(string_row)
	}

	return nil
}

func renderItem(item proto.Message) error {
	switch t := item.(type) {

	case *crypto_proto.GrrMessage:
		marshaler := &jsonpb.Marshaler{Indent: " "}
		str, err := marshaler.MarshalToString(item)
		if err != nil {
			return err
		}
		fmt.Println(str)
		fmt.Println("\nGrrMessage decodes to:")
		return renderItem(t)

	case *actions_proto.VQLResponse:
		err := renderTable(t)
		if err != nil {
			return err
		}
	default:
		marshaler := &jsonpb.Marshaler{Indent: " "}
		str, err := marshaler.MarshalToString(item)
		if err != nil {
			return err
		}
		fmt.Println(str)
	}
	return nil
}

func Inspect(config_obj *config_proto.Config, filename string) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	var urn *string
	if strings.HasPrefix(filename, "aff4:/") {
		urn = &filename
	} else {
		urn, err = datastore.FilenameToURN(config_obj, filename)
		if err != nil {
			return err
		}
	}

	item, err := getProto(*urn)
	if err != nil {
		return err
	}

	err = db.GetSubject(config_obj, *urn, item)
	if err != nil {
		return err
	}

	return renderItem(item)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {

		case inspect_command.FullCommand():
			config_obj, err := get_server_config(*config_path)
			kingpin.FatalIfError(err, "Unable to load config file")
			err = Inspect(config_obj, *inspect_filename)
			kingpin.FatalIfError(err, "Unable to parse datastore item.")

		default:
			return false
		}
		return true
	})
}
