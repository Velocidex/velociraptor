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
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
)

var (
	third_party           = app.Command("tools", "View and manipulate stored third-party binaries and tools")
	third_party_show      = third_party.Command("show", "Shows tools in the inventory")
	third_party_show_file = third_party_show.Arg("file", "Tool name to show").
				String()
	third_party_rm      = third_party.Command("rm", "Remove a third-party tool")
	third_party_rm_name = third_party_rm.Arg("name", "Tool name to remove").
				Required().String()
	third_party_upload           = third_party.Command("upload", "Upload a third-party tool")
	third_party_upload_tool_name = third_party_upload.Flag("name", "Name to assign to the tool").
					Required().String()
	third_party_upload_tool_version = third_party_upload.Flag("tool_version", "The version of the tool").String()

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

func doThirdPartyShow() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().WithRequiredFrontend().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	inventory_manager, err := services.GetInventory(config_obj)
	if err != nil {
		return err
	}

	if *third_party_show_file == "" {

		inventory := inventory_manager.Get()
		serialized, err := yaml.Marshal(inventory)
		if err != nil {
			return err
		}
		fmt.Println(string(serialized))
	} else {
		tool, err := inventory_manager.ProbeToolInfo(
			ctx, config_obj, *third_party_show_file, "")
		if err != nil {
			return fmt.Errorf("Tool not found: %w", err)
		}

		serialized, err := yaml.Marshal(tool)
		if err != nil {
			return fmt.Errorf("Serialized: %w", err)
		}
		fmt.Println(string(serialized))
	}
	return nil
}

func doThirdPartyRm() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().WithRequiredFrontend().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	inventory_manager, err := services.GetInventory(config_obj)
	if err != nil {
		return err
	}

	return inventory_manager.RemoveTool(config_obj, *third_party_rm_name)
}

func doThirdPartyUpload() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().WithRequiredFrontend().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	filename := *third_party_upload_filename
	if filename == "" {
		filename = filepath.Base(*third_party_upload_binary_path)
	}

	tool := &artifacts_proto.Tool{
		Name:         *third_party_upload_tool_name,
		Version:      *third_party_upload_tool_version,
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
		pathspec, file_store_factory, err := path_manager.Path()
		if err != nil {
			return err
		}

		writer, err := file_store_factory.WriteFile(pathspec)
		if err != nil {
			return fmt.Errorf("Unable to write to filestore: %w ", err)
		}
		defer writer.Close()

		err = writer.Truncate()
		if err != nil {
			return fmt.Errorf("Unable to write to filestore: %w ", err)
		}

		sha_sum := sha256.New()

		reader, err := os.Open(*third_party_upload_binary_path)
		if err != nil {
			return fmt.Errorf("Unable to read file: %w ", err)
		}
		defer reader.Close()

		_, err = io.Copy(writer, io.TeeReader(reader, sha_sum))
		if err != nil {
			return fmt.Errorf("Uploading file: %w", err)
		}

		tool.Hash = hex.EncodeToString(sha_sum.Sum(nil))
	}

	// Now add the tool to the inventory with the correct hash.
	inventory_manager, err := services.GetInventory(config_obj)
	if err != nil {
		return err
	}

	err = inventory_manager.AddTool(ctx,
		config_obj, tool, services.ToolOptions{
			AdminOverride: true,
		})
	if err != nil {
		return fmt.Errorf("Adding tool %s: %w", tool.Name, err)
	}

	// Materialize the tool if required
	if *third_party_upload_download {
		_, err = inventory_manager.GetToolInfo(
			ctx, config_obj, tool.Name, tool.Version)
		return err
	}

	serialized, err := yaml.Marshal(tool)
	fmt.Println(string(serialized))
	return err
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case third_party_upload.FullCommand():
			FatalIfError(third_party_upload, doThirdPartyUpload)

		case third_party_show.FullCommand():
			FatalIfError(third_party_show, doThirdPartyShow)

		case third_party_rm.FullCommand():
			FatalIfError(third_party_rm, doThirdPartyRm)

		default:
			return false
		}
		return true
	})
}
