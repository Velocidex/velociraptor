/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
// This is a file based data store. It is really simple - basically just
// writing a single file for each AFF4 object. There is no locking so
// it is not suitable for contended queues but Velociraptor does not
// use much locking any more so it should work pretty well for fairly
// large installations.

// Some limitation of this data store:

// 1. There is a small amount of overheads for small files. This
//    should not be too much but it can be reduced by using smaller
//    block sizes.
// 2. This has only been tested with a local filesystem. It may work
//    with a remote filesystem (like NFS) but performance may not be
//    that great.

// It should be safe and efficient to run multiple server instances in
// different processes since Velociraptor does not rely on locks any
// more. It is probably also wise to run the file store on the same
// filesystem.
package datastore

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/go-errors/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	file_based_imp = &FileBaseDataStore{}

	datastoreNotConfiguredError = errors.New("Datastore not configured")
)

const (
	// On windows all file paths must be prefixed by this to
	// support long paths.
	WINDOWS_LFN_PREFIX = "\\\\?\\"
)

type FileBaseDataStore struct{}

/* Gets a protobuf encoded struct from the data store.  Objects are
   addressed by the urn which is a string (URNs are typically managed
   by a path manager)
*/
func (self *FileBaseDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	defer InstrumentWithDelay("read", "FileBaseDataStore", urn)()

	Trace(config_obj, "GetSubject", urn)
	serialized_content, err := readContentFromFile(
		config_obj, urn, true /* must_exist */)
	if err != nil {
		return fmt.Errorf("While opening %v: %w", urn.AsClientPath(),
			os.ErrNotExist)
	}

	if len(serialized_content) == 0 {
		return nil
	}

	// It is really a JSON blob
	if serialized_content[0] == '{' {
		err = protojson.Unmarshal(serialized_content, message)
	} else {
		err = proto.Unmarshal(serialized_content, message)
	}

	if err != nil {
		return fmt.Errorf("While opening %v: %w",
			urn.AsClientPath(), os.ErrNotExist)
	}
	return nil
}

func (self *FileBaseDataStore) Debug(config_obj *config_proto.Config) {
	filepath.Walk(config_obj.Datastore.Location,
		func(path string, info os.FileInfo, err error) error {
			fmt.Printf("%v -> %v %v\n", path, info.Size(), info.Mode())
			return nil
		})
}

func (self *FileBaseDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	return self.SetSubjectWithCompletion(config_obj, urn, message, nil)
}

func (self *FileBaseDataStore) SetSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message, completion func()) error {

	defer InstrumentWithDelay("write", "FileBaseDataStore", urn)()

	// Make sure to call the completer on all exit points
	// (FileBaseDataStore is actually synchronous).
	defer func() {
		if completion != nil &&
			!utils.CompareFuncs(completion, utils.SyncCompleter) {
			completion()
		}
	}()

	Trace(config_obj, "SetSubject", urn)

	// Encode as JSON
	if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
		serialized_content, err := protojson.Marshal(message)
		if err != nil {
			return err
		}
		return writeContentToFile(config_obj, urn, serialized_content)
	}
	serialized_content, err := proto.Marshal(message)
	if err != nil {
		return errors.Wrap(err, 0)
	}

	return writeContentToFile(config_obj, urn, serialized_content)
}

func (self *FileBaseDataStore) DeleteSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, completion func()) error {

	err := self.DeleteSubject(config_obj, urn)
	if completion != nil &&
		!utils.CompareFuncs(completion, utils.SyncCompleter) {
		completion()
	}

	return err
}

func (self *FileBaseDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {

	defer InstrumentWithDelay("delete", "FileBaseDataStore", urn)()

	Trace(config_obj, "DeleteSubject", urn)

	err := os.Remove(urn.AsDatastoreFilename(config_obj))

	// It is ok to remove a file that does not exist.
	if err != nil && os.IsExist(err) {
		return errors.Wrap(err, 0)
	}

	// Note: We do not currently remove empty intermediate
	// directories.
	return nil
}

func listChildNames(config_obj *config_proto.Config,
	urn api.DSPathSpec) (
	[]string, error) {
	defer InstrumentWithDelay("list", "FileBaseDataStore", urn)()

	return utils.ReadDirNames(
		urn.AsDatastoreDirectory(config_obj))
}

func listChildren(config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]os.FileInfo, error) {

	defer InstrumentWithDelay("list", "FileBaseDataStore", urn)()

	children, err := utils.ReadDirUnsorted(
		urn.AsDatastoreDirectory(config_obj))
	if err != nil {
		if os.IsNotExist(err) {
			return []os.FileInfo{}, nil
		}
		return nil, errors.Wrap(err, 0)
	}

	max_dir_size := int(config_obj.Datastore.MaxDirSize)
	if max_dir_size == 0 {
		max_dir_size = 50000
	}

	if len(children) > max_dir_size {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error(
			"listChildren: Encountered a large directory %v (%v files), "+
				"truncating to %v", urn.AsClientPath(),
			len(children), max_dir_size)
		return children[:max_dir_size], nil
	}
	return children, nil
}

// Lists all the children of a URN.
func (self *FileBaseDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) (
	[]api.DSPathSpec, error) {

	TraceDirectory(config_obj, "ListChildren", urn)

	all_children, err := listChildren(config_obj, urn)
	if err != nil {
		return nil, err
	}

	// In the same directory we may have files and directories
	children := make([]os.FileInfo, 0, len(all_children))
	for _, child := range all_children {
		if strings.HasSuffix(child.Name(), ".db") || child.IsDir() {
			children = append(children, child)
		}
	}

	// Sort entries by modified time.
	sort.Slice(children, func(i, j int) bool {
		return children[i].ModTime().UnixNano() < children[j].ModTime().UnixNano()
	})

	// Slice the result according to the required offset and count.
	result := make([]api.DSPathSpec, 0, len(children))
	for _, child := range children {
		var child_pathspec api.DSPathSpec

		if child.IsDir() {
			name := utils.UnsanitizeComponent(child.Name())
			result = append(result, urn.AddUnsafeChild(name).SetDir())
			continue
		}

		// Strip data store extensions
		spec_type, extension := api.GetDataStorePathTypeFromExtension(
			child.Name())
		if spec_type == api.PATH_TYPE_DATASTORE_UNKNOWN {
			continue
		}

		name := utils.UnsanitizeComponent(child.Name()[:len(extension)])

		// Skip over files that do not belong in the data store.
		if spec_type == api.PATH_TYPE_DATASTORE_UNKNOWN {
			continue

		} else {
			child_pathspec = urn.AddUnsafeChild(name).SetType(spec_type)
		}

		result = append(result, child_pathspec)
	}

	return result, nil
}

// Called to close all db handles etc. Not thread safe.
func (self *FileBaseDataStore) Close() {}

func writeContentToFile(config_obj *config_proto.Config,
	urn api.DSPathSpec, data []byte) error {

	if config_obj.Datastore == nil {
		return datastoreNotConfiguredError
	}

	filename := urn.AsDatastoreFilename(config_obj)

	// Truncate the file immediately so we dont need to make a seocnd
	// syscall.
	file, err := os.OpenFile(
		filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)

	// Try to create intermediate directories and try again.
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(filename), 0700)
		if err != nil {
			return err
		}
		file, err = os.OpenFile(
			filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
		if err != nil {
			return err
		}
	}
	if err != nil {
		logging.GetLogger(config_obj, &logging.FrontendComponent).Error(
			"Unable to open file %v: %v", filename, err)
		return errors.Wrap(err, 0)
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return errors.Wrap(err, 0)
	}
	return nil
}

func readContentFromFile(
	config_obj *config_proto.Config, urn api.DSPathSpec,
	must_exist bool) ([]byte, error) {

	if config_obj.Datastore == nil {
		return nil, datastoreNotConfiguredError
	}

	file, err := os.Open(urn.AsDatastoreFilename(config_obj))
	if err == nil {
		defer file.Close()

		result, err := ioutil.ReadAll(
			io.LimitReader(file, constants.MAX_MEMORY))
		if err != nil {
			return nil, errors.Wrap(err, 0)
		}

		return result, nil
	}

	// Try to read older protobuf based files for backwards
	// compatibility.
	if os.IsNotExist(err) &&
		urn.Type() == api.PATH_TYPE_DATASTORE_JSON {

		file, err := os.Open(urn.
			SetType(api.PATH_TYPE_DATASTORE_PROTO).
			AsDatastoreFilename(config_obj))

		if err == nil {
			defer file.Close()

			result, err := ioutil.ReadAll(
				io.LimitReader(file, constants.MAX_MEMORY))
			if err != nil {
				return nil, errors.Wrap(err, 0)
			}

			return result, nil
		}
	}

	// Its ok if the file does not exist - no error.
	if !must_exist && os.IsNotExist(err) {
		return []byte{}, nil
	}
	return nil, errors.Wrap(err, 0)
}

// Convert a file name from the data store to a DSPathSpec
func FilenameToURN(config_obj *config_proto.Config,
	filename string) api.DSPathSpec {
	if runtime.GOOS == "windows" {
		filename = strings.TrimPrefix(filename, WINDOWS_LFN_PREFIX)
	}

	filename = strings.TrimPrefix(filename, config_obj.Datastore.Location)

	components := []string{}
	// DS filenames are always clean so a strings split is fine.
	for _, component := range strings.Split(
		filename, string(os.PathSeparator)) {
		if component != "" {
			components = append(components, component)
		}
	}

	// Strip any extension from the last component.
	if len(components) > 0 {
		last := len(components) - 1
		components[last] = strings.TrimPrefix(
			strings.TrimSuffix(components[last], ".db"), ".json")
	}

	return path_specs.NewSafeDatastorePath(components...)
}

func Trace(config_obj *config_proto.Config,
	name string, filename api.DSPathSpec) {

	return

	fmt.Printf("Trace FileBaseDataStore: %v: %v\n", name,
		filename.AsDatastoreFilename(config_obj))
}

func TraceDirectory(config_obj *config_proto.Config,
	name string, filename api.DSPathSpec) {

	return

	fmt.Printf("Trace FileBaseDataStore: %v: %v\n", name,
		filename.AsDatastoreDirectory(config_obj))
}

// Support RawDataStore interface
func (self *FileBaseDataStore) GetBuffer(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]byte, error) {

	return readContentFromFile(
		config_obj, urn, true /* must exist */)
}

func (self *FileBaseDataStore) SetBuffer(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, data []byte, completion func()) error {

	err := writeContentToFile(config_obj, urn, data)
	if completion != nil &&
		!utils.CompareFuncs(completion, utils.SyncCompleter) {
		completion()
	}
	return err
}
