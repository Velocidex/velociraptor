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
	"gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var classifiers = map[string]proto.Message{
	"aff4:/clients/C.[^/]+$":                            &actions_proto.ClientInfo{},
	"aff4:/clients/C.[^/]+/ping$":                       &actions_proto.ClientInfo{},
	"aff4:/clients/C.[^/]+/key$":                        &crypto_proto.PublicKey{},
	"aff4:/clients/C.[^/]+/vfs/.+":                      &actions_proto.VQLResponse{},
	"aff4:/clients/C.[^/]+/flows/F\\.[^/]+$":            &flows_proto.AFF4FlowObject{},
	"aff4:/clients/C.[^/]+/flows/F\\.[^/]+/results/.+$": &crypto_proto.GrrMessage{},
	"aff4:/clients/C.[^/]+/tasks/[^/]+$":                &crypto_proto.GrrMessage{},
	"aff4:/hunts/H.[^/]+$":                              &api_proto.Hunt{},
	"aff4:/users/[^/]+$":                                &api_proto.VelociraptorUser{},
	"aff4:/users/[^/]+/notifications/.+$":               &api_proto.UserNotification{},
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
			string_row = append(string_row, utils.Stringify(item, scope))
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
		return renderItem(responder.ExtractGrrMessagePayload(t))

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

func Inspect(config_obj *api_proto.Config, filename string) error {
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
