/*
  The new style API:

  1. Paths are treated as a list of components (no need for callers to
     worry about escaping).

  2. All data is written out in JSON. Although protobufs are more
     compact, JSON is easier to work with and the space saving is not
     significant (the datastore only stores very small files).

  The old style API uses URNs as strings that are converted back and
  forth from components. This will eventually be deprecated.
*/

package datastore

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	errors "github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

/*
func componentsToFilename(
	config_obj *config_proto.Config, components []string) string {

	// Sanitize all components so they are suitable for the filesystem.
	new_components := make([]string, 0, len(components))
	for _, i := range components {
		new_components = append(new_components, utils.SanitizeString(i))
	}

	// On Windows we need to:
	// 1. Use \ as path separator.
	// 2. Prefix the path with \\?\ to ensure it uses 32k path
	//    length. Otherwise the MAX_PATH is very short.

	// This relies on the filepath starting with a drive letter
	// and having \ as path separators. Main's
	// validateServerConfig() ensures this is the case.
	if runtime.GOOS == "windows" {
		return "\\\\?\\" + config_obj.Datastore.Location +
			"\\" + strings.Join(new_components, "\\")
	}

	// On linux and mac we use / as path separator.
	return config_obj.Datastore.Location + "/" +
		strings.Join(new_components, "/")
}
*/

// List a directory on disk and produce a list of valid data store
// files contained within the directory.
func (self *FileBaseDataStore) ListChildrenJSON(
	config_obj *config_proto.Config,
	path api.PathSpec) ([]*DatastoreInfo, error) {

	dirpath := path.AsDatastoreDirectory(config_obj)
	child_names, err := utils.ReadDirNames(dirpath)
	if err != nil {
		return nil, err
	}

	result := make([]*DatastoreInfo, 0, len(child_names))
	for _, name := range child_names {
		if !strings.HasSuffix(name, ".json.db") {
			continue
		}

		s, err := os.Lstat(filepath.Join(dirpath, name))
		if err != nil {
			continue
		}

		// Unsanitize the component from a filename to
		// a component name.
		component := utils.UnsanitizeComponent(
			strings.TrimSuffix(name, ".json.db"))
		result = append(result, &DatastoreInfo{
			Name:     component,
			Modified: s.ModTime(),
		})
	}

	return result, nil
}

func (self *FileBaseDataStore) GetSubjectJSON(
	config_obj *config_proto.Config,
	path api.PathSpec,
	message proto.Message) error {

	filename := path.SetType("json").
		AsDatastoreFilename(config_obj)
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// Try to read the file as an old protobuf
			// based file.
			filename = path.SetType("").
				AsDatastoreFilename(config_obj)
			file, err = os.Open(filename)
			if err != nil {
				return err
			}
			defer file.Close()

			serialized_content, err := ioutil.ReadAll(
				io.LimitReader(file, constants.MAX_MEMORY))
			if err != nil {
				return nil
			}

			// It might be an older file, try to read the
			// protobuf version.
			return proto.Unmarshal(serialized_content, message)
		}
		return err
	}
	defer file.Close()

	serialized_content, err := ioutil.ReadAll(
		io.LimitReader(file, constants.MAX_MEMORY))

	// Empty file means no subject is set.
	if len(serialized_content) == 0 {
		return nil
	}

	err = protojson.Unmarshal(serialized_content, message)
	if err != nil {
		return errors.WithMessage(os.ErrNotExist,
			fmt.Sprintf("While openning %v: %v", filename, err))
	}
	return nil
}

func (self *FileBaseDataStore) SetSubjectJSON(
	config_obj *config_proto.Config,
	path api.PathSpec,
	message proto.Message) error {

	filename := path.SetType("json").
		AsDatastoreFilename(config_obj)

	// Encode as JSON
	serialized_content, err := protojson.Marshal(message)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0660)

	// Try to create intermediate directories and try again.
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(filename), 0700)
		if err != nil {
			return err
		}
		file, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0660)
		if err != nil {
			return err
		}
	}
	if err != nil {
		logging.GetLogger(config_obj, &logging.FrontendComponent).Error(
			"Unable to open file "+filename, err)
		return errors.WithStack(err)
	}
	defer file.Close()

	err = file.Truncate(0)
	if err != nil {
		return err
	}

	_, err = file.Write(serialized_content)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (self *FileBaseDataStore) WalkComponents(
	config_obj *config_proto.Config,
	root_components api.PathSpec, walkFn ComponentWalkFunc) error {

	dirname := root_components.AsDatastoreDirectory(config_obj)
	names, err := utils.ReadDirNames(dirname)
	if err != nil {
		return err
	}

	for _, name := range names {
		s, err := os.Lstat(filepath.Join(dirname, name))
		if err != nil {
			continue
		}

		next_name := utils.UnsanitizeComponent(name)
		next_components := root_components.AddChild(next_name)

		// If it is a directory walk it as well.
		if s.IsDir() {
			err = self.WalkComponents(config_obj,
				next_components, walkFn)
			if err == filepath.SkipDir {
				return err
			}
		}

		err = walkFn(next_components)
		if err == filepath.SkipDir {
			return err
		}
	}
	return nil
}
