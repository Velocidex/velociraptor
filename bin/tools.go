package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Velocidex/yaml/v2"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/utils"
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

	third_party_cat      = third_party.Command("cat", "Dump tool from command line")
	third_party_cat_name = third_party_cat.Arg("name", "Tool name to dump").
				Required().String()

	third_party_cat_version = third_party_cat.Flag("version", "Tool version to dump").
				String()

	url_regexp = regexp.MustCompile("^https?://")
)

func doThirdPartyShow() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().WithRequiredFrontend().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := Install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return err
	}
	defer sm.Close()

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

	ctx, cancel := Install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return err
	}
	defer sm.Close()

	inventory_manager, err := services.GetInventory(config_obj)
	if err != nil {
		return err
	}

	return inventory_manager.RemoveTool(
		ctx, config_obj, *third_party_rm_name)
}

func doThirdPartyCat() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().WithRequiredFrontend().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := Install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return err
	}
	defer sm.Close()

	inventory_manager, err := services.GetInventory(config_obj)
	if err != nil {
		return err
	}

	fd, err := inventory_manager.ReadTool(
		ctx, config_obj, *third_party_cat_name, *third_party_cat_version)
	if err != nil {
		return err
	}

	_, err = utils.Copy(ctx, os.Stdout, fd)
	return err
}

func doThirdPartyUpload() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().WithRequiredFrontend().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := Install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return err
	}
	defer sm.Close()

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

	// Does the user want to scrape releases from github?
	if *third_party_upload_github_project != "" {
		tool.GithubProject = *third_party_upload_github_project
		tool.GithubAssetRegex = *third_party_upload_github_asset_regex

		// If the user wants to upload a URL we just write it in the
		// filestore to be downloaded on demand by the client themselves.
	} else if url_regexp.FindString(*third_party_upload_binary_path) != "" {
		tool.Url = *third_party_upload_binary_path

	} else {
		writer, err := inventory_manager.WriteTool(ctx, config_obj,
			tool.Name, tool.Version)
		if err != nil {
			return fmt.Errorf("Unable to write to filestore: %w ", err)
		}
		defer writer.Close()

		reader, err := os.Open(*third_party_upload_binary_path)
		if err != nil {
			return fmt.Errorf("Unable to read file: %w ", err)
		}
		defer reader.Close()

		_, err = io.Copy(writer, reader)
		if err != nil {
			return fmt.Errorf("Uploading file: %w", err)
		}
	}

	// Materialize the tool if required
	if *third_party_upload_download {
		tool, err = inventory_manager.GetToolInfo(
			ctx, config_obj, tool.Name, tool.Version)
		if err != nil {
			return err
		}
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

		case third_party_cat.FullCommand():
			FatalIfError(third_party_cat, doThirdPartyCat)

		default:
			return false
		}
		return true
	})
}
