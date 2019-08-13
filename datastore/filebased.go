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

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/velociraptor/urns"
)

type FileBaseDataStore struct {
	clock testing.Clock
}

func (self *FileBaseDataStore) GetClientTasks(
	config_obj *config_proto.Config,
	client_id string,
	do_not_lease bool) ([]*crypto_proto.GrrMessage, error) {
	result := []*crypto_proto.GrrMessage{}
	now := self.clock.Now().UTC().UnixNano() / 1000
	tasks_urn := urns.BuildURN("clients", client_id, "tasks")
	now_urn := tasks_urn + fmt.Sprintf("/%d", now)

	tasks, err := self.ListChildren(config_obj, tasks_urn, 0, 100)
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
		message := &crypto_proto.GrrMessage{}
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
	message *crypto_proto.GrrMessage) error {

	task_urn := urns.BuildURN("clients", client_id, "tasks",
		fmt.Sprintf("/%d", message.TaskId))

	return self.DeleteSubject(config_obj, task_urn)
}

func (self *FileBaseDataStore) QueueMessageForClient(
	config_obj *config_proto.Config,
	client_id string,
	flow_id string,
	client_action string,
	message proto.Message,
	next_state uint64) error {

	now := self.clock.Now().UTC().UnixNano() / 1000
	subject := urns.BuildURN("clients", client_id, "tasks",
		fmt.Sprintf("/%d", now))

	req, err := responder.NewRequest(message)
	if err != nil {
		return err
	}

	req.Name = client_action
	req.SessionId = flow_id
	req.RequestId = uint64(next_state)
	req.TaskId = uint64(now)

	value, err := proto.Marshal(req)
	if err != nil {
		return errors.WithStack(err)
	}

	return writeContentToFile(config_obj, subject, value)
}

func (self *FileBaseDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn string,
	message proto.Message) error {

	serialized_content, err := readContentFromFile(
		config_obj, urn, false /* must_exist */)
	if err != nil {
		return err
	}

	if strings.HasSuffix(urn, ".json") {
		return jsonpb.UnmarshalString(
			string(serialized_content), message)
	}

	return proto.Unmarshal(serialized_content, message)
}

func (self *FileBaseDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn string,
	message proto.Message) error {

	// Encode as JSON
	if strings.HasSuffix(urn, ".json") {
		marshaler := &jsonpb.Marshaler{Indent: " "}
		serialized_content, err := marshaler.MarshalToString(
			message)
		if err != nil {
			return err
		}
		return writeContentToFile(
			config_obj, urn, []byte(serialized_content))
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
	children, err := ioutil.ReadDir(dirname)
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
	result := []string{}

	children, err := listChildren(config_obj, urn)
	if err != nil {
		return result, err
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].ModTime().Unix() > children[j].ModTime().Unix()
	})

	urn = strings.TrimSuffix(urn, "/")
	for i := offset; i < offset+length; i++ {
		if i >= uint64(len(children)) {
			break
		}

		name := UnsanitizeComponent(children[i].Name())
		if !strings.HasSuffix(name, ".db") {
			continue
		}
		result = append(
			result,
			urn+"/"+strings.TrimSuffix(name, ".db"))
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
		err := writeContentToFile(config_obj, subject, []byte{})
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
	offset uint64, limit uint64) []string {
	seen := make(map[string]bool)
	result := []string{}

	query = strings.ToLower(query)
	if query == "." || query == "" {
		query = "all"
	}

	add_func := func(key string) {
		children, err := listChildren(config_obj,
			path.Join(index_urn, key))
		if err != nil {
			return
		}

		for _, child_urn := range children {
			name := strings.TrimSuffix(
				UnsanitizeComponent(child_urn.Name()), ".db")
			seen[name] = true

			if uint64(len(seen)) > offset+limit {
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
		for _, set := range sets {
			name := strings.TrimSuffix(
				UnsanitizeComponent(set.Name()), ".db")

			matched, err := path.Match(query, name)
			if err != nil {
				// Can only happen if pattern is invalid.
				return result
			}
			if matched {
				if query_type == "key" {
					seen[name] = true
				} else {
					add_func(name)
				}
			}

			if uint64(len(seen)) > offset+limit {
				break
			}
		}
	} else {
		add_func(query)
	}

	for k := range seen {
		result = append(result, k)
	}

	if uint64(len(result)) < offset {
		return []string{}
	}

	if uint64(len(result))-offset < limit {
		limit = uint64(len(result)) - offset
	}

	return result[offset : offset+limit]
}

// Called to close all db handles etc. Not thread safe.
func (self *FileBaseDataStore) Close() {}

func init() {
	db := FileBaseDataStore{
		clock: testing.RealClock{},
	}

	RegisterImplementation("FileBaseDataStore", &db)
}

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

	result := make([]rune, len(component)*4)
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
	if config_obj.Datastore.Location == "" {
		return "", errors.New("No Datastore_location is set in the config.")
	}

	components := []string{config_obj.Datastore.Location}
	for idx, component := range strings.Split(urn, "/") {
		if idx == 0 && component == "aff4:" {
			continue
		}

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
		os.MkdirAll(filepath.Dir(filename), 0700)
		file, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0660)
	}
	if err != nil {
		logging.GetLogger(config_obj, &logging.FrontendComponent).Error(
			"Unable to open file "+filename, err)
		return errors.WithStack(err)
	}
	defer file.Close()

	file.Truncate(0)

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
	if err != nil {
		if !must_exist && os.IsNotExist(err) {
			return []byte{}, nil
		}
		return nil, errors.WithStack(err)
	}
	defer file.Close()

	result, err := ioutil.ReadAll(
		io.LimitReader(file, constants.MAX_MEMORY))
	return result, errors.WithStack(err)
}

// Convert a file name from the data store to a urn.
func FilenameToURN(config_obj *config_proto.Config, filename string) (*string, error) {
	if config_obj.Datastore.Implementation != "FileBaseDataStore" {
		return nil, errors.New("Unsupported data store")
	}

	if !strings.HasPrefix(filename, config_obj.Datastore.Location) {
		return nil, errors.New("Filename is not within the FileBaseDataStore location.")
	}

	location := strings.TrimSuffix(config_obj.Datastore.Location, "/")
	components := []string{}
	for _, component := range strings.Split(
		strings.TrimPrefix(filename, location), "/") {
		components = append(components, UnsanitizeComponent(component))
	}

	result := strings.TrimSuffix("aff4:"+strings.Join(components, "/"), ".db")
	return &result, nil
}
