/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/golang/protobuf/ptypes/empty"
	errors "github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	file_based_imp = &FileBaseDataStore{
		clock: utils.RealClock{},
	}

	g_id uint64
)

const (
	// On windows all file paths must be prefixed by this to
	// support long paths.
	WINDOWS_LFN_PREFIX = "\\\\?\\"
)

type FileBaseDataStore struct {
	clock utils.Clock
}

func (self *FileBaseDataStore) GetClientTasks(
	config_obj *config_proto.Config,
	client_id string,
	do_not_lease bool) ([]*crypto_proto.VeloMessage, error) {
	result := []*crypto_proto.VeloMessage{}

	client_path_manager := paths.NewClientPathManager(client_id)
	tasks, err := self.ListChildren(
		config_obj, client_path_manager.TasksDirectory(), 0, 100)
	if err != nil {
		return nil, err
	}

	for _, task_urn := range tasks {
		// Here we read the task from the task_urn and remove
		// it from the queue.
		message := &crypto_proto.VeloMessage{}
		err = self.GetSubject(config_obj, task_urn, message)
		if err != nil {
			continue
		}

		if !do_not_lease {
			err = self.DeleteSubject(config_obj, task_urn)
			if err != nil {
				return nil, err
			}
		}
		result = append(result, message)
	}
	return result, nil
}

func (self *FileBaseDataStore) UnQueueMessageForClient(
	config_obj *config_proto.Config,
	client_id string,
	message *crypto_proto.VeloMessage) error {

	client_path_manager := paths.NewClientPathManager(client_id)
	return self.DeleteSubject(config_obj,
		client_path_manager.Task(message.TaskId))
}

func (self *FileBaseDataStore) currentTaskId() uint64 {
	id := atomic.AddUint64(&g_id, 1)
	return uint64(self.clock.Now().UTC().UnixNano()&0x7fffffffffff0000) | (id & 0xFFFF)
}

func (self *FileBaseDataStore) QueueMessageForClient(
	config_obj *config_proto.Config,
	client_id string,
	req *crypto_proto.VeloMessage) error {

	// Task ID is related to time.
	req.TaskId = self.currentTaskId()

	client_path_manager := paths.NewClientPathManager(client_id)
	return self.SetSubject(config_obj,
		client_path_manager.Task(req.TaskId), req)
}

/* Gets a protobuf encoded struct from the data store.  Objects are
   addressed by the urn which is a string (URNs are typically managed
   by a path manager)
*/
func (self *FileBaseDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn api.PathSpec,
	message proto.Message) error {

	serialized_content, err := readContentFromFile(
		config_obj, urn, true /* must_exist */)
	if err != nil {
		// Second try the old DB without json. This support
		// migration from old protobuf based datastore files
		// to newer json based blobs while still being able to
		// read old files.
		if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
			serialized_content, err = readContentFromFile(
				config_obj,
				urn.SetType(api.PATH_TYPE_DATASTORE_PROTO),
				true /* must_exist */)
		}
		if err != nil {
			return errors.WithMessage(os.ErrNotExist,
				fmt.Sprintf("While openning %v: %v",
					urn.AsClientPath(), err))
		}
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
		return errors.WithMessage(os.ErrNotExist,
			fmt.Sprintf("While openning %v: %v", urn, err))
	}
	return nil
}

func (self *FileBaseDataStore) Walk(config_obj *config_proto.Config,
	root api.PathSpec, walkFn WalkFunc) error {

	return filepath.Walk(root.AsDatastoreDirectory(config_obj),
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			// We are only interested in filenames that end with .db
			basename := info.Name()
			if !strings.HasSuffix(basename, ".db") {
				return nil
			}

			urn := FilenameToURN(config_obj, path)
			return walkFn(urn)
		})
}

func (self *FileBaseDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn api.PathSpec,
	message proto.Message) error {

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
		return errors.WithStack(err)
	}

	return writeContentToFile(config_obj, urn, serialized_content)
}

func (self *FileBaseDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.PathSpec) error {

	err := os.Remove(urn.AsDatastoreFilename(config_obj))

	// It is ok to remove a file that does not exist.
	if err != nil && os.IsExist(err) {
		return errors.WithStack(err)
	}

	// Note: We do not currently remove empty intermediate
	// directories.
	return nil
}

func listChildNames(config_obj *config_proto.Config,
	urn api.PathSpec) (
	[]string, error) {
	return utils.ReadDirNames(
		urn.AsDatastoreDirectory(config_obj))
}

func listChildren(config_obj *config_proto.Config,
	urn api.PathSpec) ([]os.FileInfo, error) {
	children, err := utils.ReadDirUnsorted(
		urn.AsDatastoreDirectory(config_obj))
	if err != nil {
		if os.IsNotExist(err) {
			return []os.FileInfo{}, nil
		}
		return nil, errors.WithStack(err)
	}
	return children, nil
}

// Lists all the children of a URN.
func (self *FileBaseDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn api.PathSpec,
	offset uint64, length uint64) (
	[]api.PathSpec, error) {
	all_children, err := listChildren(config_obj, urn)
	if err != nil {
		return nil, err
	}

	// In the same directory we may have files and directories -
	// in here we only care about the files which have a .db
	// extension so filter the directory listing.
	children := make([]os.FileInfo, 0, len(all_children))
	for _, child := range all_children {
		if strings.HasSuffix(child.Name(), ".db") {
			children = append(children, child)
		}
	}

	// Sort entries by modified time.
	sort.Slice(children, func(i, j int) bool {
		return children[i].ModTime().UnixNano() < children[j].ModTime().UnixNano()
	})

	// Slice the result according to the required offset and count.
	result := make([]api.PathSpec, 0, len(children))
	for i := offset; i < offset+length; i++ {
		if i >= uint64(len(children)) {
			break
		}

		// Strip data store extensions
		name := children[i].Name()
		name = strings.TrimSuffix(name, ".db")
		name = strings.TrimSuffix(name, ".json")
		utils.Debug(name)
		result = append(result,
			urn.AddChild(utils.UnsanitizeComponent(name)))
	}

	return result, nil
}

// Update the posting list index. Searching for any of the
// keywords will return the entity urn.
func (self *FileBaseDataStore) SetIndex(
	config_obj *config_proto.Config,
	index_urn api.PathSpec,
	entity string,
	keywords []string) error {

	entity = utils.SanitizeString(entity)

	for _, keyword := range keywords {
		// The entity and keywords are not trusted because
		// they are user provided.
		keyword = utils.SanitizeString(strings.ToLower(keyword))
		subject := index_urn.AddChild(keyword, entity)
		err := self.SetSubject(config_obj, subject, &empty.Empty{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *FileBaseDataStore) UnsetIndex(
	config_obj *config_proto.Config,
	index_urn api.PathSpec,
	entity string,
	keywords []string) error {

	entity = utils.SanitizeString(entity)

	for _, keyword := range keywords {
		keyword = utils.SanitizeString(strings.ToLower(keyword))
		subject := index_urn.AddChild(keyword, entity)
		err := self.DeleteSubject(config_obj, subject)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *FileBaseDataStore) CheckIndex(
	config_obj *config_proto.Config,
	index_urn api.PathSpec,
	entity string,
	keywords []string) error {

	entity = utils.SanitizeString(entity)

	for _, keyword := range keywords {
		keyword = utils.SanitizeString(strings.ToLower(keyword))
		subject := index_urn.AddChild(keyword, entity)
		_, err := readContentFromFile(
			config_obj, subject, true /* must_exist */)
		// Any of the matching entities means we checked
		// successfully.
		if err == nil {
			return nil
		}
	}
	return errors.New("Client does not have label")
}

func (self *FileBaseDataStore) SearchClients(
	config_obj *config_proto.Config,
	index_urn api.PathSpec,
	query string, query_type string,
	offset uint64, limit uint64, sort_direction SortingSense) []string {

	seen := make(map[string]bool)

	var result []string

	if sort_direction == UNSORTED {
		result = make([]string, 0, offset+limit)
	}

	query = strings.ToLower(query)
	if query == "." || query == "" {
		query = "all"
	}

	// If the result set is not sorted we can quit as soon as we
	// have enough results. When sorting the results we are forced
	// to enumerate all the clients, sort them and then chop them
	// up into pages.
	can_quit_early := func() bool {
		return limit > 0 && sort_direction == UNSORTED &&
			uint64(len(result)) > offset+limit
	}

	add_func := func(key string) {
		children, err := listChildNames(config_obj,
			index_urn.AddChild(utils.SanitizeString(key)))
		if err != nil {
			return
		}

		for _, child_name := range children {
			_, pres := seen[child_name]
			if !pres {
				seen[child_name] = true
				name := utils.UnsanitizeComponent(child_name)
				name = strings.TrimSuffix(name, ".db")
				result = append(result, name)
			}

			if can_quit_early() {
				break
			}
		}
	}

	// Query has a wildcard.
	if strings.ContainsAny(query, "[]*?") {
		// We could make it smarter in future but this is
		// quick enough for now.
		sets, err := listChildNames(config_obj, index_urn)
		if err != nil {
			return result
		}

		if sort_direction != UNSORTED {
			result = make([]string, 0, len(sets))
		}

		for _, set := range sets {
			name := utils.UnsanitizeComponent(set)
			name = strings.TrimSuffix(name, ".db")
			matched, err := path.Match(query, name)
			if err != nil {
				// Can only happen if pattern is invalid.
				return result
			}
			if matched {
				if query_type == "key" {
					_, pres := seen[name]
					if !pres {
						seen[name] = true
						result = append(result, name)
					}
				} else {
					add_func(name)
				}
			}

			if can_quit_early() {
				break
			}
		}
	} else {
		add_func(query)
	}

	// No results within the range.
	if uint64(len(result)) < offset {
		return []string{}
	}

	// Sort the search results for stable pagination output.
	switch sort_direction {
	case SORT_DOWN:
		sort.Slice(result, func(i, j int) bool {
			return result[i] > result[j]
		})
	case SORT_UP:
		sort.Slice(result, func(i, j int) bool {
			return result[i] < result[j]
		})
	}

	// Clamp the limit to the end of the results we have.
	if limit > 0 {
		if uint64(len(result))-offset < limit {
			limit = uint64(len(result)) - offset
		}

		return result[offset : offset+limit]
	}

	return result[offset:]
}

// Called to close all db handles etc. Not thread safe.
func (self *FileBaseDataStore) Close() {}

func writeContentToFile(config_obj *config_proto.Config,
	urn api.PathSpec, data []byte) error {

	filename := urn.AsDatastoreFilename(config_obj)
	file, err := os.OpenFile(
		filename, os.O_RDWR|os.O_CREATE, 0660)

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

	_, err = file.Write(data)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func readContentFromFile(
	config_obj *config_proto.Config, urn api.PathSpec,
	must_exist bool) ([]byte, error) {
	file, err := os.Open(urn.AsDatastoreFilename(config_obj))
	if err == nil {
		defer file.Close()

		result, err := ioutil.ReadAll(
			io.LimitReader(file, constants.MAX_MEMORY))
		return result, errors.WithStack(err)
	}

	// Its ok if the file does not exist - no error.
	if !must_exist && os.IsNotExist(err) {
		return []byte{}, nil
	}
	return nil, errors.WithStack(err)
}

// Convert a file name from the data store to a SafeDatastorePath
func FilenameToURN(config_obj *config_proto.Config,
	filename string) api.PathSpec {
	if runtime.GOOS == "windows" {
		filename = strings.TrimPrefix(filename, WINDOWS_LFN_PREFIX)
	}

	filename = strings.TrimPrefix(
		filename, config_obj.Datastore.FilestoreDirectory)

	components := []string{}
	for _, component := range strings.Split(
		filename, string(os.PathSeparator)) {
		component = strings.TrimSuffix(component, ".db")
		if component != "" {
			components = append(components, component)
		}
	}
	return api.NewSafeDatastorePath(components...)
}
