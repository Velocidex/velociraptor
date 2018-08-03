// Inspect the file store and decode the stored objects.
package inspect

import (
	"encoding/json"
	"fmt"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/olekukonko/tablewriter"
	errors "github.com/pkg/errors"
	"os"
	"regexp"
	"strings"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

var classifiers = map[string]proto.Message{
	"aff4:/C.[^/]+$":                              &actions_proto.ClientInfo{},
	"aff4:/C.[^/]+/ping$":                         &actions_proto.ClientInfo{},
	"aff4:/C.[^/]+/key$":                          &crypto_proto.PublicKey{},
	"aff4:/C.[^/]+/vfs/.+":                        &actions_proto.VQLResponse{},
	"aff4:/C.[^/]+/flows/F\\.[^\\.]+":             &flows_proto.AFF4FlowObject{},
	"aff4:/C.[^/]+/flows/F\\.[^\\.]+/results/.+$": &actions_proto.VQLResponse{},
	"aff4:/C.[^/]+/tasks/[^\\.]+$":                &crypto_proto.GrrMessage{},
	"aff4:/hunts/H.[^/]+$":                        &api_proto.Hunt{},
	"aff4:/hunts/H.[^/]+/(results|pending|" +
		"completed|running)/C.[^/]+$": &api_proto.HuntInfo{},
}

func getProto(urn string) (proto.Message, error) {
	for k, v := range classifiers {
		m, err := regexp.MatchString(k, urn)
		if m && err == nil {
			return v, nil
		}
	}

	return nil, errors.New("Unknown URN pattern")
}

func hard_wrap(text string, colBreak int) string {
	text = strings.TrimSpace(text)
	wrapped := ""
	var i int
	for i = 0; len(text[i:]) > colBreak; i += colBreak {

		wrapped += text[i:i+colBreak] + "\n"

	}
	wrapped += text[i:]

	return wrapped
}

func renderTable(response *actions_proto.VQLResponse) error {
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
			string_row = append(string_row, fmt.Sprintf("%v", item))
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

func Inspect(config_obj *config.Config, filename string) error {
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
