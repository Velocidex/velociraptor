package datastore

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	gTestDatastore = NewTestDataStore()
)

type TestDataStore struct {
	mu sync.Mutex

	Subjects    map[string]proto.Message
	ClientTasks map[string][]*crypto_proto.GrrMessage
}

func NewTestDataStore() *TestDataStore {
	return &TestDataStore{
		Subjects:    make(map[string]proto.Message),
		ClientTasks: make(map[string][]*crypto_proto.GrrMessage),
	}
}

func (self *TestDataStore) Get(path string) proto.Message {
	self.mu.Lock()
	defer self.mu.Unlock()

	result, _ := self.Subjects[path]
	return result
}

func (self *TestDataStore) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.Subjects = make(map[string]proto.Message)
	self.ClientTasks = make(map[string][]*crypto_proto.GrrMessage)
}

func (self *TestDataStore) Debug() {
	result := []string{}

	for k, v := range self.Subjects {
		result = append(result, fmt.Sprintf("%v: %v", k, string(
			json.MustMarshalIndent(v))))
	}

	fmt.Println(strings.Join(result, "\n"))
}

func (self *TestDataStore) GetClientTasks(config_obj *config_proto.Config,
	client_id string,
	do_not_lease bool) ([]*crypto_proto.GrrMessage, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := self.ClientTasks[client_id]
	if !do_not_lease {
		delete(self.ClientTasks, client_id)
	}
	return result, nil
}

func (self *TestDataStore) Walk(
	config_obj *config_proto.Config,
	root string, walkFn WalkFunc) error {
	return nil
}

func (self *TestDataStore) QueueMessageForClient(
	config_obj *config_proto.Config,
	client_id string,
	message *crypto_proto.GrrMessage) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	result, pres := self.ClientTasks[client_id]
	if !pres {
		result = make([]*crypto_proto.GrrMessage, 0)
	}

	result = append(result, message)

	self.ClientTasks[client_id] = result
	return nil
}

func (self *TestDataStore) UnQueueMessageForClient(
	config_obj *config_proto.Config,
	client_id string,
	message *crypto_proto.GrrMessage) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	old_queue, pres := self.ClientTasks[client_id]
	if !pres {
		old_queue = make([]*crypto_proto.GrrMessage, 0)
	}

	new_queue := make([]*crypto_proto.GrrMessage, len(old_queue))
	for _, item := range old_queue {
		if message.TaskId != item.TaskId {
			new_queue = append(new_queue, item)
		}
	}

	self.ClientTasks[client_id] = new_queue
	return nil
}

func (self *TestDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn string,
	message proto.Message) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := self.Subjects[urn]
	if result != nil {
		proto.Merge(message, result)
	}
	return nil
}

func (self *TestDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn string,
	message proto.Message) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.Subjects[urn] = message

	return nil
}

func (self *TestDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.Subjects, urn)

	return nil
}

// Lists all the children of a URN.
func (self *TestDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn string,
	offset uint64, length uint64) ([]string, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []string{}
	names, err := self.listChildren(config_obj, urn, offset, length)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		result = append(result, urn+"/"+name)
	}
	end := offset + length
	if end > uint64(len(result)) {
		end = uint64(len(result))
	}

	sort.Strings(result)

	return result[offset:end], nil
}

func (self *TestDataStore) listChildren(
	config_obj *config_proto.Config,
	urn string,
	offset uint64, length uint64) ([]string, error) {

	result := []string{}

	for k := range self.Subjects {
		if strings.HasPrefix(k, urn) {
			k = strings.TrimLeft(strings.TrimPrefix(k, urn), "/")
			components := strings.Split(k, "/")
			if len(components) > 0 &&
				!utils.InString(result, components[0]) {
				result = append(result, components[0])
			}
		}
	}

	return result, nil
}

// Update the posting list index. Searching for any of the
// keywords will return the entity urn.
func (self *TestDataStore) SetIndex(
	config_obj *config_proto.Config,
	index_urn string,
	entity string,
	keywords []string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, keyword := range keywords {
		subject := path.Join(index_urn, strings.ToLower(keyword), entity)
		self.Subjects[subject] = &empty.Empty{}
	}
	return nil
}

func (self *TestDataStore) UnsetIndex(
	config_obj *config_proto.Config,
	index_urn string,
	entity string,
	keywords []string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, keyword := range keywords {
		subject := path.Join(index_urn, strings.ToLower(keyword), entity)
		delete(self.Subjects, subject)
	}
	return nil
}

func (self *TestDataStore) CheckIndex(
	config_obj *config_proto.Config,
	index_urn string,
	entity string,
	keywords []string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, keyword := range keywords {
		subject := path.Join(index_urn, strings.ToLower(keyword), entity)
		_, pres := self.Subjects[subject]
		if pres {
			return nil
		}
	}
	return errors.New("Client does not have label")
}

func (self *TestDataStore) SearchClients(
	config_obj *config_proto.Config,
	index_urn string,
	query string, query_type string,
	offset uint64, limit uint64, sort_direction SortingSense) []string {
	seen := make(map[string]bool)
	result := []string{}

	self.mu.Lock()
	defer self.mu.Unlock()

	query = strings.ToLower(query)
	if query == "." || query == "" {
		query = "all"
	}

	add_func := func(key string) {
		children, err := self.listChildren(config_obj,
			path.Join(index_urn, key), 0, 1000)
		if err != nil {
			return
		}

		for _, child_urn := range children {
			name := path.Base(child_urn)
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
		sets, err := self.listChildren(config_obj, index_urn, 0, 1000)
		if err != nil {
			return result
		}
		for _, set := range sets {
			name := path.Base(set)
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

	return result[offset : offset+limit]
}

// Called to close all db handles etc. Not thread safe.
func (self *TestDataStore) Close() {
	mu.Lock()
	defer mu.Unlock()

	gTestDatastore = NewTestDataStore()
}
