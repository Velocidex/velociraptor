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

	var allowed_tree, denied_tree *utils.PrefixTree

	if len(config_obj.Security.AllowedFileAccessorPrefix) > 0 {
		allowed_tree = utils.NewPrefixTree(case_insensitive)
		for _, allowed := range config_obj.Security.AllowedFileAccessorPrefix {
			full_path, err := accessors.NewNativePath(allowed)
			if err != nil {
				continue
			}

			allowed_tree.Add(full_path.Components)
		}
	}

	if len(config_obj.Security.DeniedFileAccessorPrefix) > 0 {
		denied_tree = utils.NewPrefixTree(case_insensitive)
		for _, denied := range config_obj.Security.DeniedFileAccessorPrefix {
			full_path, err := accessors.NewNativePath(denied)
			if err != nil {
				continue
			}

			denied_tree.Add(full_path.Components)
		}
	}

	// Clear the initial state
	file.SetPrefixes(allowed_tree, denied_tree)

	// Now handle the fs accessor
	allowed_tree = nil
	denied_tree = nil

	// Load default set of FS accessor prefixs
	if len(config_obj.Security.AllowedFsAccessorPrefix) > 0 {
		allowed_tree = utils.NewPrefixTree(false)
		for _, allowed := range config_obj.Security.AllowedFileAccessorPrefix {
			full_path, err := accessors.NewFileStorePath(allowed)
			if err != nil {
				continue
			}

			allowed_tree.Add(full_path.Components)
		}
	}

	if len(config_obj.Security.DeniedFsAccessorPrefix) > 0 {
		denied_tree = utils.NewPrefixTree(false)
		for _, denied := range config_obj.Security.DeniedFsAccessorPrefix {
			full_path, err := accessors.NewFileStorePath(denied)
			if err != nil {
				continue
			}

			denied_tree.Add(full_path.Components)
		}
	}

	file_store.SetPrefixes(allowed_tree, denied_tree)

	// Populate any additional environ vars that need to be shadowed.
	for _, s := range config_obj.Security.ShadowedEnvVars {
		common.ShadowedEnv = append(common.ShadowedEnv, s)
	}

	return nil
}
