package main

import (
	"fmt"
	"io/ioutil"
	"os"

	jsonpatch "github.com/evanphx/json-patch/v5"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

var (
	config_show_command_merge = config_show_command.Flag(
		"merge", "Merge this json config into the generated config (see https://datatracker.ietf.org/doc/html/rfc7396)").
		Strings()

	config_show_command_merge_file = config_show_command.Flag(
		"merge_file", "Merge this file containing a json config into the generated config (see https://datatracker.ietf.org/doc/html/rfc7396)").
		File()

	config_show_command_patch = config_show_command.Flag(
		"patch", "Patch this into the generated config (see http://jsonpatch.com/)").
		Strings()

	config_show_command_patch_file = config_show_command.Flag(
		"patch_file", "Patch this file into the generated config (see http://jsonpatch.com/)").
		File()
)

func applyMergesAndPatches(
	config_obj *config_proto.Config,
	merge_file *os.File, merges []string,
	patch_file *os.File, json_patches []string) error {
	// First apply merge patches
	for _, merge_patch := range getMergePatches(merge_file, merges) {
		serialized, err := json.Marshal(config_obj)
		if err != nil {
			return fmt.Errorf("Marshal config_obj: %w", err)
		}

		patched, err := jsonpatch.MergePatch(
			serialized, []byte(merge_patch))
		if err != nil {
			return fmt.Errorf("Invalid merge patch: %w", err)
		}

		err = json.Unmarshal(patched, &config_obj)
		if err != nil {
			return fmt.Errorf(
				"Patched object produces an invalid config (%v): %w",
				merge_patch, err)
		}
	}

	// Now apply json patches
	patches, err := getJsonPatches(patch_file, json_patches)
	if err != nil {
		return err
	}
	for _, patch := range patches {
		serialized, err := json.Marshal(config_obj)
		if err != nil {
			return fmt.Errorf("Marshal config_obj: %w", err)
		}

		patched, err := patch.Apply(serialized)
		if err != nil {
			return fmt.Errorf("Unable to apply JSON patch: %w", err)
		}

		err = json.Unmarshal(patched, &config_obj)
		if err != nil {
			return fmt.Errorf(
				"Patched object produces an invalid config (%v): %w",
				patch, err)
		}
	}
	return nil
}

func getMergePatches(merge_file *os.File, merges []string) [][]byte {
	result := make([][]byte, 0)
	if *config_generate_command_merge_file != nil {
		data, _ := ioutil.ReadAll(merge_file)
		result = append(result, data)
	}

	for _, merge := range merges {
		result = append(result, []byte(merge))
	}

	return result
}

func getJsonPatches(patch_file *os.File, patches []string) ([]jsonpatch.Patch, error) {
	result := make([]jsonpatch.Patch, 0)
	if *config_generate_command_patch_file != nil {
		data, err := ioutil.ReadAll(patch_file)
		if err != nil {
			return nil, fmt.Errorf("Reading patch file: %w", err)
		}

		// Parse the json patch
		patch, err := jsonpatch.DecodePatch(data)
		if err != nil {
			return nil, fmt.Errorf("Decoding JSON patch: %w", err)
		}

		result = append(result, patch)
	}

	for _, patch_data := range patches {
		// Parse the json patch
		patch, err := jsonpatch.DecodePatch([]byte(patch_data))
		if err != nil {
			return nil, fmt.Errorf("Decoding JSON patch: %w", err)
		}

		result = append(result, patch)
	}

	return result, nil
}
