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

	"github.com/golang/protobuf/ptypes/empty"
	errors "github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

var (
	file_based_imp = &FileBaseDataStore{
		clock: vtesting.RealClock{},
	}
)

const (
	// On windows all file paths must be prefixed by this to
	// support long paths.
	WINDOWS_LFN_PREFIX = "\\\\?\\"
)

type FileBaseDataStore struct {
	clock vtesting.Clock
}

func (self *FileBaseDataStore) GetClientTasks(
	config_obj *config_proto.Config,
	client_id string,
	do_not_lease bool) ([]*crypto_proto.VeloMessage, error) {
	result := []*crypto_proto.VeloMessage{}
	now := uint64(self.clock.Now().UTC().UnixNano() / 1000)

	client_path_manager := paths.NewClientPathManager(client_id)
	now_urn := client_path_manager.Task(now).Path()

	tasks, err := self.ListChildren(
		config_obj, client_path_manager.TasksDirectory().Path(), 0, 100)
	if err != nil {
		return nil, err
	}

	for _, task_urn := range tasks {
		// Only read until the current timestamp.
		if task_urn > now_urn {
			break
		}

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
		client_path_manager.Task(message.TaskId).Path())
}

func (self *FileBaseDataStore) QueueMessageForClient(
	config_obj *config_proto.Config,
	client_id string,
	req *crypto_proto.VeloMessage) error {

	req.TaskId = uint64(self.clock.Now().UTC().UnixNano() / 1000)
	client_path_manager := paths.NewClientPathManager(client_id)
	return self.SetSubject(config_obj,
		client_path_manager.Task(req.TaskId).Path(), req)
}

/* Gets a protobuf encoded struct from the data store.  Objects are
   addressed by the urn which is a string (URNs are typically managed
   by a path manager)

   FIXME: Refactor GetSubject to accept path manager directly.
*/
func (self *FileBaseDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn string,
	message proto.Message) error {

	serialized_content, err := readContentFromFile(
		config_obj, urn, true /* must_exist */)
	if err != nil {
		return errors.WithMessage(os.ErrNotExist,
			fmt.Sprintf("While openning %v: %v", urn, err))
	}

	if strings.HasSuffix(urn, ".json") {
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
	root string, walkFn WalkFunc) error {
	root_path, err := urnToFilename(config_obj, root)
	if err != nil {
		return err
	}

	root_path = strings.TrimSuffix(root_path, ".db")

	return filepath.Walk(root_path,
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

			urn, err := FilenameToURN(config_obj, path)
			if err != nil {
				return err
			}

			return walkFn(urn)
		})
}

func (self *FileBaseDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn string,
	message proto.Message) error {

	// Encode as JSON
	if strings.HasSuffix(urn, ".json") {
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
	urn string) error {

	filename, err := urnToFilename(config_obj, urn)
	if err != nil {
		return err
	}
	err = os.Remove(filename)

	// It is ok to remove a file that does not exist.
	if err != nil && os.IsExist(err) {
		return errors.WithStack(err)
	}

	// Note: We do not currently remove empty intermediate
	// directories.
	return nil
}

func listChildren(config_obj *config_proto.Config,
	urn string) ([]os.FileInfo, error) {
	filename, err := urnToFilename(config_obj, urn)
	if err != nil {
		return nil, err
	}
	dirname := strings.TrimSuffix(filename, ".db")
	children, err := utils.ReadDirUnsorted(dirname)
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
	urn string,
	offset uint64, length uint64) ([]string, error) {
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
	result := make([]string, 0, len(children))
	urn = strings.TrimSuffix(urn, "/")
	for i := offset; i < offset+length; i++ {
		if i >= uint64(len(children)) {
			break
		}

		name := UnsanitizeComponent(children[i].Name())
		name = strings.TrimSuffix(name, ".db")
		result = append(result, utils.PathJoin(urn, name, "/"))
	}

	return result, nil
}

// Update the posting list index. Searching for any of the
// keywords will return the entity urn.
func (self *FileBaseDataStore) SetIndex(
	config_obj *config_proto.Config,
	index_urn string,
	entity string,
	keywords []string) error {

	for _, keyword := range keywords {
		subject := path.Join(index_urn, strings.ToLower(keyword), entity)
		err := self.SetSubject(config_obj, subject, &empty.Empty{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *FileBaseDataStore) UnsetIndex(
	config_obj *config_proto.Config,
	index_urn string,
	entity string,
	keywords []string) error {

	for _, keyword := range keywords {
		subject := path.Join(index_urn, strings.ToLower(keyword), entity)
		err := self.DeleteSubject(config_obj, subject)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *FileBaseDataStore) CheckIndex(
	config_obj *config_proto.Config,
	index_urn string,
	entity string,
	keywords []string) error {
	for _, keyword := range keywords {
		subject := path.Join(index_urn, strings.ToLower(keyword), entity)
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
	index_urn string,
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
		return sort_direction == UNSORTED &&
			uint64(len(result)) > offset+limit
	}

	add_func := func(key string) {
		children, err := listChildren(config_obj,
			path.Join(index_urn, key))
		if err != nil {
			return
		}

		for _, child_urn := range children {
			name := UnsanitizeComponent(child_urn.Name())
			name = strings.TrimSuffix(name, ".db")
			_, pres := seen[name]
			if !pres {
				seen[name] = true
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
		sets, err := listChildren(config_obj, index_urn)
		if err != nil {
			return result
		}

		if sort_direction != UNSORTED {
			result = make([]string, 0, len(sets))
		}

		for _, set := range sets {
			name := UnsanitizeComponent(set.Name())
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
	if uint64(len(result))-offset < limit {
		limit = uint64(len(result)) - offset
	}

	return result[offset : offset+limit]
}

// Called to close all db handles etc. Not thread safe.
func (self *FileBaseDataStore) Close() {}

var hexTable = []rune("0123456789ABCDEF")

// We are very conservative about our escaping.
func shouldEscape(c byte) bool {
	if 'A' <= c && c <= 'Z' ||
		'a' <= c && c <= 'z' ||
		'0' <= c && c <= '9' {
		return false
	}

	switch c {
	case '-', '_', '.', '~', ' ', '$':
		return false
	}

	return true
}

func SanitizeString(component string) []rune {
	// Escape components that start with . - these are illegal on
	// windows.
	if len(component) > 0 && component[0:1] == "." {
		return []rune("%2E" + component[1:])
	}

	// Prevent components from creating names for files that are
	// used internally by the datastore.
	if strings.HasSuffix(component, ".db") {
		component += "_"
	}

	length := len(component)
	if length > 1024 {
		length = 1024
	}

	result := make([]rune, length*4)
	result_idx := 0

	for _, c := range []byte(component) {
		if !shouldEscape(c) {
			result[result_idx] = rune(c)
			result_idx += 1
		} else {
			result[result_idx] = '%'
			result[result_idx+1] = hexTable[c>>4]
			result[result_idx+2] = hexTable[c&15]
			result_idx += 3
		}
	}
	return result[:result_idx]
}

func unhex(c rune) rune {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

func UnsanitizeComponent(component_str string) string {
	component := []rune(component_str)
	result := ""
	i := 0
	for {
		if i >= len(component) {
			return result
		}

		if component[i] == '%' {
			c := unhex(component[i+1])<<4 | unhex(component[i+2])
			result += string(c)
			i += 3
		} else {
			result += string(component[i])
			i += 1
		}
	}
}

func urnToFilename(config_obj *config_proto.Config, urn string) (string, error) {
	if config_obj.Datastore == nil ||
		config_obj.Datastore.Location == "" {
		return "", errors.New("No Datastore_location is set in the config.")
	}

	components := []string{config_obj.Datastore.Location}
	for _, component := range utils.SplitComponents(urn) {
		components = append(components, string(SanitizeString(component)))
	}

	// Files all end with .db. Note a component can never have
	// this suffix so it is not possible to break the datastore by
	// having a urn with a component that ends with .db which
	// creates a directory with the same name as a file.
	result := filepath.Join(components...) + ".db"

	// This relies on the filepath starting with a drive letter
	// and having \ as path separators. Main's
	// validateServerConfig() ensures this is the case.
	if runtime.GOOS == "windows" {
		return "\\\\?\\" + result, nil
	}

	// fmt.Printf("Accessing on %v\n", result)

	return result, nil
}

func writeContentToFile(config_obj *config_proto.Config, urn string, data []byte) error {
	filename, err := urnToFilename(config_obj, urn)
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

	_, err = file.Write(data)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func readContentFromFile(
	config_obj *config_proto.Config, urn string,
	must_exist bool) ([]byte, error) {
	filename, err := urnToFilename(config_obj, urn)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(filename)
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

// Convert a file name from the data store to a urn.
func FilenameToURN(config_obj *config_proto.Config, filename string) (string, error) {
	if runtime.GOOS == "windows" {
		filename = strings.TrimPrefix(filename, WINDOWS_LFN_PREFIX)
	}

	filename = strings.TrimPrefix(
		filename, config_obj.Datastore.FilestoreDirectory)

	components := []string{}
	for _, component := range strings.Split(
		filename,
		string(os.PathSeparator)) {
		component = strings.TrimSuffix(component, ".db")
		components = append(components,
			string(UnsanitizeComponent(component)))
	}

	// Filestore filenames always use / as separator.
	result := utils.JoinComponents(components, "/")
	return result, nil
}
