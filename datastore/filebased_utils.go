package datastore

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	sep = string(filepath.Separator)
)

func AsFilestoreFilename(
	db DataStore, config_obj *config_proto.Config, path api.FSPathSpec) string {

	return AsFilestoreDirectory(db, config_obj, path) +
		api.GetExtensionForFilestore(path)
}

func AsFilestoreDirectory(
	db DataStore, config_obj *config_proto.Config,
	path api.FSPathSpec) string {
	data_store_root := ""
	if config_obj != nil && config_obj.Datastore != nil {
		data_store_root = config_obj.Datastore.FilestoreDirectory
	}

	if path.IsSafe() {
		return asSafeDirWithRoot(path.AsDatastorePath(), data_store_root)
	}
	return asUnsafeDirWithRoot(db, config_obj,
		path.AsDatastorePath(), data_store_root)
}

func AsDatastoreDirectory(
	db DataStore, config_obj *config_proto.Config, path api.DSPathSpec) string {
	location := ""
	if config_obj.Datastore != nil {
		location = config_obj.Datastore.Location
	}

	if path.IsSafe() {
		return asSafeDirWithRoot(path, location)
	}
	return asUnsafeDirWithRoot(db, config_obj, path, location)
}

// When we are unsafe we need to Sanitize components hitting the
// filesystem.
func asUnsafeDirWithRoot(
	db DataStore,
	config_obj *config_proto.Config,
	path api.DSPathSpec, root string) string {
	new_components := make([]string, 0, len(path.Components()))
	for _, i := range path.Components() {
		if i != "" {
			new_components = append(new_components, CompressComponent(
				db, config_obj, i))
		}
	}
	result := ""
	if len(new_components) > 0 {
		result = sep + strings.Join(new_components, sep)
	}

	// This relies on the filepath starting with a drive letter
	// and having \ as path separators. Main's
	// validateServerConfig() ensures this is the case.
	if runtime.GOOS == "windows" {
		return "\\\\?\\" + root + result
	}
	return root + result
}

func AsDatastoreFilename(
	db DataStore, config_obj *config_proto.Config, path api.DSPathSpec) string {
	return AsDatastoreDirectory(db, config_obj, path) +
		api.GetExtensionForDatastore(path)
}

// If the path spec is already safe we can shortcut it and not
// sanitize. Safe paths are assumed to be generated from within the
// application and therefore can not overflow path lengths - so we
// dont need to compress them.
func asSafeDirWithRoot(
	path api.DSPathSpec, root string) string {
	// No need to sanitize here because the DSPathSpec is already
	// safe.
	sep := string(filepath.Separator)

	// This relies on the filepath starting with a drive letter
	// and having \ as path separators, and having a trailing
	// \. Main's config.ValidateDatastoreConfig() ensures this is
	// the case.
	if runtime.GOOS == "windows" {
		// Remove empty components which are broken on windows due to
		// the long filename hack.
		components := make([]string, 0, len(path.Components()))
		for _, c := range path.Components() {
			if c != "" {
				components = append(components, c)
			}
		}
		return WINDOWS_LFN_PREFIX + root + sep +
			strings.Join(components, sep)
	}

	return root + sep + strings.Join(path.Components(), sep)
}

// This function is only really called when listing a directory - we
// find the hash compressed member and need to reconstruct its full
// name.
func UncompressComponent(
	db DataStore,
	config_obj *config_proto.Config,
	component string) string {
	if len(component) == 0 || component[0] != '#' {
		return utils.UnsanitizeComponent(component)
	}

	ds_pathspec := &api_proto.DSPathSpec{}
	err := db.GetSubject(config_obj,
		LFNCompressedHashPath(component), ds_pathspec)
	if err != nil || len(ds_pathspec.Components) != 1 {
		return component
	}

	return ds_pathspec.Components[0]
}

// Sanitize the component for the filesystem. If the component name is
// too long we also compress it by replacing it with a hash and
// storing the long path is a different area of the datastore. The
// result is that we can transparently write files with arbitrarily
// long paths, regardless of the capabilities of the underlying
// filesystem.
func CompressComponent(
	db DataStore,
	config_obj *config_proto.Config,
	component string) string {
	sanitized_component := utils.SanitizeString(component)
	if len(sanitized_component) < 250 {
		return sanitized_component
	}

	return compressComponent(db, config_obj, component)
}

func compressComponent(
	db DataStore,
	config_obj *config_proto.Config,
	component string) string {

	// Hash compress the original component
	hash := sha1.Sum([]byte(component))
	component_hash := fmt.Sprintf("#%x", hash)
	db, err := GetDB(config_obj)
	if err != nil {
		return component_hash
	}

	ds_pathspec := &api_proto.DSPathSpec{
		Components: []string{component},
	}
	_ = db.SetSubject(config_obj,
		LFNCompressedHashPath(component_hash), ds_pathspec)
	return component_hash
}

// Long file names are compressed into hashes and stored in the
// datastore.
func LFNCompressedHashPath(hash string) api.DSPathSpec {
	res := path_specs.NewSafeDatastorePath("lfn_hashes").
		SetType(api.PATH_TYPE_DATASTORE_JSON)

	i := 0
	var part []byte
	for _, c := range []byte(hash) {
		if c == '#' {
			continue
		}
		part = append(part, c)
		i++

		if i == 3 || i == 12 {
			res = res.AddChild(string(part))
			part = nil
		}
	}

	if len(part) > 0 {
		res = res.AddChild(string(part))
	}

	return res
}

// Ensure all intermediate directories exist.
func MkdirAll(
	db DataStore,
	config_obj *config_proto.Config,
	dirname api.FSPathSpec) error {

	file_path := AsFilestoreFilename(db, config_obj,
		dirname.SetType(api.PATH_TYPE_DATASTORE_DIRECTORY))
	return os.MkdirAll(file_path, 0700)
}
