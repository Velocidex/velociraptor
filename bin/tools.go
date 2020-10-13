package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Velocidex/yaml/v2"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	third_party           = app.Command("tools", "Manipulate third party binaries and tools")
	third_party_show      = third_party.Command("show", "Upload a third party binary")
	third_party_show_file = third_party_show.Arg("file", "Upload a third party binary").
				String()
	third_party_rm      = third_party.Command("rm", "Remove a third party binary")
	third_party_rm_name = third_party_rm.Arg("name", "The name to remove").
				Required().String()
	third_party_upload           = third_party.Command("upload", "Upload a third party binary")
	third_party_upload_tool_name = third_party_upload.Flag("name", "Name of the tool").
					Required().String()
	third_party_upload_filename = third_party_upload.
					Flag("filename", "Name of the tool executable on the endpoint").
					String()

	third_party_upload_github_project = third_party_upload.
						Flag("github_project",
			"Fetch the tool for github releases").String()
	third_party_upload_github_asset_regex = third_party_upload.
						Flag("github_asset",
			"A regular expression to match the release asset").String()

	third_party_upload_serve_remote = third_party_upload.Flag(
		"serve_remote", "If set serve the file from the original URL").Bool()

	third_party_upload_download = third_party_upload.Flag(
		"download", "Force immediate download if set, "+
			"default - lazy download on demand").Bool()

	third_party_upload_binary_path = third_party_upload.
					Arg("path", "Path to file or a URL").String()

	url_regexp = regexp.MustCompile("^https?://")
)

func doThirdPartyShow() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().
		LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")

	sm, err := startEssentialServices(config_obj)
	kingpin.FatalIfError(err, "Starting services.")
	defer sm.Close()

	if *third_party_show_file == "" {
		inventory := services.GetInventory().Get()
		serialized, err := yaml.Marshal(inventory)
		kingpin.FatalIfError(err, "Serialized ")
		fmt.Println(string(serialized))
	} else {
		tool, err := services.GetInventory().ProbeToolInfo(*third_party_show_file)
		kingpin.FatalIfError(err, "Tool not found ")

		serialized, err := yaml.Marshal(tool)
		kingpin.FatalIfError(err, "Serialized ")
		fmt.Println(string(serialized))
	}
}

func doThirdPartyRm() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().
		LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")

	sm, err := startEssentialServices(config_obj)
	kingpin.FatalIfError(err, "Starting services.")
	defer sm.Close()

	err = services.GetInventory().RemoveTool(config_obj, *third_party_rm_name)
	kingpin.FatalIfError(err, "Removing tool ")
}

func doThirdPartyUpload() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().
		LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")

	sm, err := startEssentialServices(config_obj)
	kingpin.FatalIfError(err, "Starting services.")
	defer sm.Close()

	filename := *third_party_upload_filename
	if filename == "" {
		filename = filepath.Base(*third_party_upload_binary_path)
	}

	tool := &artifacts_proto.Tool{
		Name:         *third_party_upload_tool_name,
		Filename:     filename,
		ServeLocally: !*third_party_upload_serve_remote,
	}

	// Does the user want to scrape releases from github?
	if *third_party_upload_github_project != "" {
		tool.GithubProject = *third_party_upload_github_project
		tool.GithubAssetRegex = *third_party_upload_github_asset_regex

		// If the user wants to upload a URL we just write it in the
		// filestore to be downloaded on demand by the client themselves.
	} else if url_regexp.FindString(*third_party_upload_binary_path) != "" {
		tool.Url = *third_party_upload_binary_path

	} else {
		// Figure out where we need to store the tool.
		path_manager := paths.NewInventoryPathManager(config_obj, tool)
		file_store_factory := file_store.GetFileStore(config_obj)
		writer, err := file_store_factory.WriteFile(path_manager.Path())
		kingpin.FatalIfError(err, "Unable to write to filestore ")
		defer writer.Close()

		err = writer.Truncate()
		kingpin.FatalIfError(err, "Unable to write to filestore ")

		sha_sum := sha256.New()

		reader, err := os.Open(*third_party_upload_binary_path)
		kingpin.FatalIfError(err, "Unable to read file ")
		defer reader.Close()

		_, err = io.Copy(writer, io.TeeReader(reader, sha_sum))
		kingpin.FatalIfError(err, "Uploading file ")

		tool.Hash = hex.EncodeToString(sha_sum.Sum(nil))
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	// Now add the tool to the inventory with the correct hash.
	err = services.GetInventory().AddTool(
		config_obj, tool, services.ToolOptions{
			AdminOverride: true,
		})
	kingpin.FatalIfError(err, "Adding tool "+tool.Name)

	if *third_party_upload_download {
		tool, err = services.GetInventory().GetToolInfo(ctx, config_obj, tool.Name)
		kingpin.FatalIfError(err, "Fetching file ")
	}

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
