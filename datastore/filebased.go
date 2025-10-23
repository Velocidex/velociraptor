/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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

// This is a file based data store. It is the default data store we
// use for both small and large deployments. The datastore is only
// used for storing local metadata and uses no locking:

// Object IO is considered atomic - there are no locks. This can
// result in races for contentended objects but the Velociraptor
// design avoids file contention at all times.

// Files can be written as protobuf encoding (this is the old
// standard) but this makes it harder to debug so now most files will
// be written as json encoded blobs. There is fallback code to be able
// to read only protobuf encoded files if they are there but newer
// files will be written as JSON.

package datastore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/go-errors/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	file_based_imp = &FileBaseDataStore{}

	datastoreNotConfiguredError = errors.New("Datastore not configured")
	invalidFileError            = errors.New("Invalid file error")
	insufficientDiskSpace       = errors.New("Insufficient disk space!")
)

const (
	// On windows all file paths must be prefixed by this to
	// support long paths.
	WINDOWS_LFN_PREFIX = "\\\\?\\"
)

type FileBaseDataStore struct {
	mu  sync.Mutex
	err error
}

/*
Gets a protobuf encoded struct from the data store.  Objects are
addressed by the urn (URNs are typically managed by a path manager)
*/
func (self *FileBaseDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	defer InstrumentWithDelay("read", "FileBaseDataStore", urn)()

	Trace(self, config_obj, "GetSubject", urn)
	serialized_content, err := readContentFromFile(self, config_obj, urn)
	if err != nil {
		return fmt.Errorf("While opening %v: %w", urn.AsClientPath(),
			utils.NotFoundError)
	}

	if len(serialized_content) == 0 {
		// JSON encoded files must contain at least '{}' two
		// characters. If the file is empty something went wrong -
		// usually this is because the disk was full.
		if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
			return fmt.Errorf("While accessing %v: %w",
				urn.AsClientPath(), invalidFileError)
		}
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
			urn.AsClientPath(), utils.NotFoundError)
	}
	return nil
}

func (self *FileBaseDataStore) Debug(config_obj *config_proto.Config) {
	_ = filepath.Walk(config_obj.Datastore.Location,
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

	err := self.Healthy()
	if err != nil {
		return err
	}

	// Make sure to call the completer on all exit points
	// (FileBaseDataStore is actually synchronous).
	defer func() {
		if completion != nil &&
			!utils.CompareFuncs(completion, utils.SyncCompleter) {
			completion()
		}
	}()

	Trace(self, config_obj, "SetSubject", urn)

	// Encode as JSON
	if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
		serialized_content, err := protojson.Marshal(message)
		if err != nil {
			return err
		}
		return writeContentToFile(self, config_obj, urn, serialized_content)
	}
	serialized_content, err := proto.Marshal(message)
	if err != nil {
		return errors.Wrap(err, 0)
	}

	return writeContentToFile(self, config_obj, urn, serialized_content)
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

	Trace(self, config_obj, "DeleteSubject", urn)

	err := os.Remove(AsDatastoreFilename(self, config_obj, urn))

	// It is ok to remove a file that does not exist.
	if err != nil && os.IsExist(err) {
		return errors.Wrap(err, 0)
	}

	// Note: We do not currently remove empty intermediate
	// directories.
	return nil
}

func (self *FileBaseDataStore) listChildren(config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]os.FileInfo, error) {

	defer InstrumentWithDelay("list", "FileBaseDataStore", urn)()

	children, err := utils.ReadDirUnsorted(
		AsDatastoreDirectory(self, config_obj, urn))
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

	TraceDirectory(self, config_obj, "ListChildren", urn)

	all_children, err := self.listChildren(config_obj, urn)
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

	db, err := GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	// Slice the result according to the required offset and count.
	result := make([]api.DSPathSpec, 0, len(children))
	for _, child := range children {
		var child_pathspec api.DSPathSpec

		if child.IsDir() {
			name := UncompressComponent(db, config_obj, child.Name())
			result = append(result, urn.AddUnsafeChild(name).SetDir())
			continue
		}

		// Strip data store extensions
		spec_type, extension := api.GetDataStorePathTypeFromExtension(
			child.Name())
		if spec_type == api.PATH_TYPE_DATASTORE_UNKNOWN {
			continue
		}

		name := UncompressComponent(db,
			config_obj, child.Name()[:len(extension)])

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

func writeContentToFile(
	db DataStore, config_obj *config_proto.Config,
	urn api.DSPathSpec, data []byte) error {

	if config_obj.Datastore == nil {
		return datastoreNotConfiguredError
	}

	filename := AsDatastoreFilename(db, config_obj, urn)

	// Truncate the file immediately so we dont need to make a seocnd
	// syscall. Empirically on Linux, a truncate call always works,
	// even if there is no available disk space to accommodate the
	// required file size. This means we can not avoid file corruption
	// when the disk is full! We may as well truncate to 0 on open and
	// hope the file write succeeds later.
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
	db DataStore, config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]byte, error) {

	if config_obj.Datastore == nil {
		return nil, datastoreNotConfiguredError
	}

	file, err := os.Open(AsDatastoreFilename(db, config_obj, urn))
	if err == nil {
		defer file.Close()

		result, err := utils.ReadAllWithLimit(file, constants.MAX_MEMORY)
		if err != nil {
			return nil, errors.Wrap(err, 0)
		}

		return result, nil
	}

	// Try to read older protobuf based files for backwards
	// compatibility.
	if os.IsNotExist(err) &&
		urn.Type() == api.PATH_TYPE_DATASTORE_JSON {

		file, err := os.Open(AsDatastoreFilename(
			db, config_obj, urn.SetType(api.PATH_TYPE_DATASTORE_PROTO)))

		if err == nil {
			defer file.Close()

			result, err := utils.ReadAllWithLimit(file, constants.MAX_MEMORY)
			if err != nil {
				return nil, errors.Wrap(err, 0)
			}

			return result, nil
		}
	}
	return nil, errors.Wrap(err, 0)
}

func Trace(
	db DataStore,
	config_obj *config_proto.Config,
	name string, filename api.DSPathSpec) {

	return

	//fmt.Printf("Trace FileBaseDataStore: %v: %v\n", name,
	//	AsDatastoreFilename(db, config_obj, filename))
}

func TraceDirectory(
	db DataStore, config_obj *config_proto.Config,
	name string, filename api.DSPathSpec) {

	return

	//fmt.Printf("Trace FileBaseDataStore: %v: %v\n", name,
	//	AsDatastoreDirectory(db, config_obj, filename))
}

// Support RawDataStore interface
func (self *FileBaseDataStore) GetBuffer(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]byte, error) {

	return readContentFromFile(self, config_obj, urn)
}

func (self *FileBaseDataStore) Healthy() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.err
}

func (self *FileBaseDataStore) SetError(err error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.err = err
}

func (self *FileBaseDataStore) SetBuffer(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, data []byte, completion func()) error {

	err := self.Healthy()
	if err != nil {
		return err
	}

	err = writeContentToFile(self, config_obj, urn, data)
	if completion != nil &&
		!utils.CompareFuncs(completion, utils.SyncCompleter) {
		completion()
	}
	return err
}
