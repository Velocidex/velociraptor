package sanity

import (
	"runtime"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/accessors/file_store"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/common"
)

func (self *SanityChecks) CheckSecuritySettings(
	config_obj *config_proto.Config) error {

	if config_obj.Security == nil {
		config_obj.Security = &config_proto.Security{}
	}

	case_insensitive := false
	// On windows we must use case insensitive match.
	if runtime.GOOS == "windows" {
		case_insensitive = true
	}

	if len(config_obj.Security.AllowedFileAccessorPrefix) > 0 {
		file.AllowedPrefixes = utils.NewPrefixTree(case_insensitive)
		for _, allowed := range config_obj.Security.AllowedFileAccessorPrefix {
			full_path, err := accessors.NewNativePath(allowed)
			if err != nil {
				continue
			}

			file.AllowedPrefixes.Add(full_path.Components)
		}
	}

	if len(config_obj.Security.DeniedFileAccessorPrefix) > 0 {
		file.DeniedPrefixes = utils.NewPrefixTree(case_insensitive)
		for _, denied := range config_obj.Security.DeniedFileAccessorPrefix {
			full_path, err := accessors.NewNativePath(denied)
			if err != nil {
				continue
			}

			file.DeniedPrefixes.Add(full_path.Components)
		}
	}

	// Load default set of FS accessor prefixs
	if len(config_obj.Security.AllowedFsAccessorPrefix) == 0 {
		config_obj.Security.AllowedFsAccessorPrefix = []string{
			"artifact_definitions",
			"clients",
			"downloads",
			"notebooks",
			"public",
			"temp",
			"server_artifacts",
			"server_artifacts_logs",
		}
	}

	if file_store.AllowedPrefixes == nil {
		file_store.AllowedPrefixes = utils.NewPrefixTree(case_insensitive)
	}
	for _, allowed := range config_obj.Security.AllowedFsAccessorPrefix {
		file_store.AllowedPrefixes.Add([]string{allowed})
	}

	// Populate any additional environ vars that need to be shadowed.
	for _, s := range config_obj.Security.ShadowedEnvVars {
		common.ShadowedEnv = append(common.ShadowedEnv, s)
	}

	return nil
}
