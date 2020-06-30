package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/Velocidex/yaml/v2"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	third_party         = app.Command("third_party", "Manipulate third party binaries")
	third_party_show    = third_party.Command("show", "Upload a third party binary")
	third_party_rm      = third_party.Command("rm", "Remove a third party binary")
	third_party_rm_name = third_party_rm.Arg("name", "The name to remove").
				Required().String()
	third_party_upload           = third_party.Command("upload", "Upload a third party binary")
	third_party_upload_tool_name = third_party_upload.Flag("name", "Name of the tool").
					Required().String()
	third_party_upload_serve_remote = third_party_upload.Flag(
		"serve_remote", "If set serve the file from the original URL").Bool()
	third_party_upload_binary_path = third_party_upload.
					Arg("path", "Path to file or a URL").String()
)

func doThirdPartyShow() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().
		LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")

	wg := &sync.WaitGroup{}
	err = services.StartInventoryService(context.Background(), wg, config_obj)
	kingpin.FatalIfError(err, "Load Config ")

	serialized, err := yaml.Marshal(services.Inventory.Get())
	kingpin.FatalIfError(err, "Serialized ")
	fmt.Println(string(serialized))
}

func doThirdPartyRm() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().
		LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")

	wg := &sync.WaitGroup{}
	err = services.StartInventoryService(context.Background(), wg, config_obj)
	kingpin.FatalIfError(err, "Load Config ")

	err = services.Inventory.RemoveTool(config_obj, *third_party_rm_name)
	kingpin.FatalIfError(err, "Removing tool ")
}

func doThirdPartyUpload() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().
		LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")

	wg := &sync.WaitGroup{}
	err = services.StartInventoryService(context.Background(), wg, config_obj)

	tool := &api_proto.Tool{
		Name:         *third_party_upload_tool_name,
		Filename:     path.Base(*third_party_upload_binary_path),
		ServeLocally: !*third_party_upload_serve_remote,
	}

	// If the user wants to upload a URL we just write it in the
	// filestore to be downloaded on demand.
	if strings.HasPrefix(*third_party_upload_binary_path, "https://") {
		tool.Url = *third_party_upload_binary_path

	} else {
		// Figure out where we need to store the tool.
		path_manager := paths.NewInventoryPathManager(config_obj, tool)
		file_store_factory := file_store.GetFileStore(config_obj)
		writer, err := file_store_factory.WriteFile(path_manager.Path())
		kingpin.FatalIfError(err, "Unable to write to filestore ")
		defer writer.Close()

		writer.Truncate()

		sha_sum := sha256.New()

		reader, err := os.Open(*third_party_upload_binary_path)
		kingpin.FatalIfError(err, "Unable to read file ")
		defer reader.Close()

		_, err = io.Copy(writer, io.TeeReader(reader, sha_sum))
		kingpin.FatalIfError(err, "Uploading file ")

		tool.Hash = hex.EncodeToString(sha_sum.Sum(nil))
	}

	// Now add the tool to the inventory with the correct hash.
	err = services.Inventory.AddTool(config_obj, tool)
	kingpin.FatalIfError(err, "Adding tool ")

	serialized, err := yaml.Marshal(tool)
	kingpin.FatalIfError(err, "Serialized ")
	fmt.Println(string(serialized))
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case third_party_upload.FullCommand():
			doThirdPartyUpload()

		case third_party_show.FullCommand():
			doThirdPartyShow()

		case third_party_rm.FullCommand():
			doThirdPartyRm()

		default:
			return false
		}
		return true
	})
}
