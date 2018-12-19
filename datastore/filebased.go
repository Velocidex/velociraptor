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
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
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
	config_obj *api_proto.Config,
	client_id string,
	do_not_lease bool) ([]*crypto_proto.GrrMessage, error) {
	result := []*crypto_proto.GrrMessage{}
	now := self.clock.Now().UTC().UnixNano() / 1000
	tasks_urn := urns.BuildURN("clients", client_id, "tasks")
	now_urn := tasks_urn + fmt.Sprintf("/%d", now)

	next_timestamp := self.clock.Now().Add(
		time.Second*time.Duration(config_obj.Frontend.ClientLeaseTime)).
		UTC().UnixNano() / 1000

	tasks, err := self.ListChildren(config_obj, tasks_urn, 0, 100)
	if err != nil {
		return nil, err
	}

	for _, task_urn := range tasks {
		// Only read until the current timestamp.
		if task_urn > now_urn {
			break
		}

		// Here we read the task from the task_urn, modify it
		// to reflect the next_timestamp and then write it to
		// a new next_timestamp_urn. When the client replies
		// to this task we can remove the next_timestamp_urn.
		message := &crypto_proto.GrrMessage{}
		err = self.GetSubject(config_obj, task_urn, message)
		if err != nil {
			continue
		}

		if !do_not_lease {
			next_timestamp_urn := tasks_urn + fmt.Sprintf(
				"/%d", next_timestamp)

			message.TaskId = uint64(next_timestamp)
			err = self.SetSubject(config_obj, next_timestamp_urn, message)
			if err != nil {
				continue
			}

			err = self.DeleteSubject(config_obj, task_urn)
			if err != nil {
				return nil, err
			}
		}
		result = append(result, message)

		// Make sure next_timestamp is unique for all messages.
		next_timestamp += 1
	}
	return result, nil
}

// Removes the task ids from the client queues.
func (self *FileBaseDataStore) RemoveTasksFromClientQueue(
	config_obj *api_proto.Config,
	client_id string,
	task_ids []uint64) error {
	for _, task_id := range task_ids {
		urn := urns.BuildURN("clients", client_id, "tasks",
			fmt.Sprintf("/%d", task_id))
		err := self.DeleteSubject(config_obj, urn)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *FileBaseDataStore) QueueMessageForClient(
	config_obj *api_proto.Config,
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
	config_obj *api_proto.Config,
	urn string,
	message proto.Message) error {

	serialized_content, err := readContentFromFile(config_obj, urn)
	if err != nil {
		return err
	}
	err = proto.Unmarshal(serialized_content, message)
	if err != nil {
		return err
	}

	return nil
}

func (self *FileBaseDataStore) SetSubject(
	config_obj *api_proto.Config,
	urn string,
	message proto.Message) error {
	serialized_content, err := proto.Marshal(message)
	if err != nil {
		return errors.WithStack(err)
	}

	return writeContentToFile(config_obj, urn, serialized_content)
}

func (self *FileBaseDataStore) DeleteSubject(
	config_obj *api_proto.Config,
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

// Lists all the children of a URN.
func (self *FileBaseDataStore) ListChildren(
	config_obj *api_proto.Config,
	urn string,
	offset uint64, length uint64) ([]string, error) {
	result := []string{}
	filename, err := urnToFilename(config_obj, urn)
	if err != nil {
		return nil, err
	}
	dirname := strings.TrimSuffix(filename, ".db")
	children, err := ioutil.ReadDir(dirname)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, errors.WithStack(err)
	}

	sort.Slice(children, func(i, j int) bool {
		return children[i].ModTime().Unix() > children[j].ModTime().Unix()
	})

	urn = strings.TrimSuffix(urn, "/")
	for i := offset; i < offset+length; i++ {
		if i >= uint64(len(children)) {
			break
		}
		component := strings.TrimSuffix(
			UnsanitizeComponent(children[i].Name()), ".db")

		// If there is both a file and directory refering to
		// the same component we will have it twice so skip
		// duplicates.
		child_urn := urn + "/" + component
		if len(result) > 0 && result[len(result)-1] == child_urn {
			continue
		}
		result = append(result, child_urn)
	}
	return result, nil
}

// Update the posting list index. Searching for any of the
// keywords will return the entity urn.
func (self *FileBaseDataStore) SetIndex(
	config_obj *api_proto.Config,
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

func (self *FileBaseDataStore) SearchClients(
	config_obj *api_proto.Config,
	index_urn string,
	query string,
	offset uint64, limit uint64) []string {
	result := []string{}

	query = strings.ToLower(query)
	if query == "." || query == "" {
		query = "all"
	}

	children, err := self.ListChildren(
		config_obj, path.Join(index_urn, query), offset, limit)
	if err != nil {
		return result
	}

	for _, child_urn := range children {
		client_id := path.Base(child_urn)
		if strings.HasPrefix(client_id, "C.") {
			result = append(result, client_id)
		}
	}

	return result
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
func shouldEscape(c rune) bool {
	if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' {
		return false
	}

	switch c {
	case '-', '_', '.', '~':
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

	result := make([]rune, len(component)*3)
	result_idx := 0

	for _, c := range component {
		if !shouldEscape(c) {
			result[result_idx] = c
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

func urnToFilename(config_obj *api_proto.Config, urn string) (string, error) {
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
	return path.Join(components...) + ".db", nil
}

func writeContentToFile(config_obj *api_proto.Config, urn string, data []byte) error {
	filename, err := urnToFilename(config_obj, urn)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0660)
	// Try to create intermediate directories and try again.
	if err != nil && os.IsNotExist(err) {
		os.MkdirAll(path.Dir(filename), 0700)
		file, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0660)
	}
	if err != nil {
		logging.NewLogger(config_obj).Error(
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

func readContentFromFile(config_obj *api_proto.Config, urn string) ([]byte, error) {
	filename, err := urnToFilename(config_obj, urn)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte{}, nil
		}
		return nil, errors.WithStack(err)
	}
	defer file.Close()

	result, err := ioutil.ReadAll(file)
	return result, errors.WithStack(err)
}

// Convert a file name from the data store to a urn.
func FilenameToURN(config_obj *api_proto.Config, filename string) (*string, error) {
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
