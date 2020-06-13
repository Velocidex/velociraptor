package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/paths"
)

var (
	third_party                  = app.Command("third_party", "Manipulate third party binaries")
	third_party_upload           = third_party.Command("upload", "Upload a third party binary")
	third_party_upload_tool_name = third_party_upload.Flag("name", "Name of the tool").
					Required().String()
	third_party_upload_binary_path = third_party_upload.Arg("path", "Path to file").
					ExistingFile()
)

func getInventory(config_obj *config_proto.Config) ([]*ordereddict.Dict, error) {
	result := []*ordereddict.Dict{}

	file_store_factory := file_store.GetFileStore(config_obj)
	inventory_path_manager := paths.NewInventoryPathManager()
	rows, err := directory.ReadRowsCSV(context.Background(),
		file_store_factory, inventory_path_manager.Path(), 0, 0)
	if err != nil {
		return nil, err
	}

	for row := range rows {
		result = append(result, row)
	}

	return result, nil
}

func setInventory(config_obj *config_proto.Config, inventory []*ordereddict.Dict) error {
	file_store_factory := file_store.GetFileStore(config_obj)
	inventory_path_manager := paths.NewInventoryPathManager()

	writer, err := file_store_factory.WriteFile(inventory_path_manager.Path())
	if err != nil {
		return err
	}
	defer writer.Close()

	writer.Truncate()

	if len(inventory) == 0 {
		return nil
	}

	csv_writer := csv.NewWriter(writer)
	csv_writer.Write(inventory[0].Keys())
	defer csv_writer.Flush()

	for _, row := range inventory {
		fmt.Printf("%v\n", row)
		csv_row := []string{}
		for _, key := range row.Keys() {
			v, _ := row.Get(key)
			csv_row = append(csv_row, csv.AnyToString(v))
		}
		csv_writer.Write(csv_row)
	}

	return nil
}

func doThirdPartyUpload() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().
		LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")

	filename := path.Base(*third_party_upload_binary_path)

	path_manager := paths.NewThirdPartyPathManager(filename)
	file_store_factory := file_store.GetFileStore(config_obj)
	writer, err := file_store_factory.WriteFile(path_manager.Path())
	kingpin.FatalIfError(err, "Unable to write to filestore ")
	defer writer.Close()

	writer.Truncate()

	sha_sum := sha256.New()

	reader, err := os.Open(*third_party_upload_binary_path)
	kingpin.FatalIfError(err, "Unable to read file ")
	defer reader.Close()

	buf := make([]byte, 1024*1024)
	for {
		n, err := reader.Read(buf)
		if n == 0 {
			break
		}
		if err != nil && err != io.EOF {
			kingpin.FatalIfError(err, "Unable to write to filestore ")
		}

		data := buf[:n]

		writer.Write(data)
		sha_sum.Write(data)
	}

	// If we can not read the inventory we make a new one.
	inventory, _ := getInventory(config_obj)

	new_inventory := []*ordereddict.Dict{}

	for _, row := range inventory {
		tool, _ := row.GetString("Tool")
		if tool == *third_party_upload_tool_name {
			continue
		}

		new_inventory = append(new_inventory, row)
	}

	new_inventory = append(new_inventory, ordereddict.NewDict().
		Set("Tool", *third_party_upload_tool_name).
		Set("Type", ".").
		Set("Filename", filename).
		Set("ExpectedHash", hex.EncodeToString(sha_sum.Sum(nil))))

	err = setInventory(config_obj, new_inventory)
	kingpin.FatalIfError(err, "Unable to write to inventory ")
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case third_party_upload.FullCommand():
			doThirdPartyUpload()

		default:
			return false
		}
		return true
	})
}
