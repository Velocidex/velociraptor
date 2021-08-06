package datastore

/*
   An in-memory data store for tests.
*/

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"

	errors "github.com/pkg/errors"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	gTestDatastore = NewTestDataStore()
)

type TestDataStore struct {
	mu sync.Mutex

	idx         uint64
	Subjects    map[string]proto.Message
	Components  map[string][]string
	ClientTasks map[string][]*crypto_proto.VeloMessage

	clock utils.Clock
}

func NewTestDataStore() *TestDataStore {
	return &TestDataStore{
		Subjects:    make(map[string]proto.Message),
		Components:  make(map[string][]string),
		ClientTasks: make(map[string][]*crypto_proto.VeloMessage),
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
	self.Components = make(map[string][]string)
	self.ClientTasks = make(map[string][]*crypto_proto.VeloMessage)
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
	do_not_lease bool) ([]*crypto_proto.VeloMessage, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := self.ClientTasks[client_id]
	if !do_not_lease {
		delete(self.ClientTasks, client_id)
	}
	return result, nil
}

// If child_components are a subpath of parent_components (i.e. are
// parent_components is an exact prefix of child_components)
func isSubPath(parent_components []string, child_components []string) bool {
	if len(parent_components) > len(child_components) {
		return false
	}

	for i := 0; i < len(parent_components); i++ {
		if parent_components[i] != child_components[i] {
			return false
		}
	}
	return true
}

func (self *TestDataStore) Walk(
	config_obj *config_proto.Config,
	root api.DSPathSpec, walkFn WalkFunc) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	root_components := root.Components()
	for k := range self.Subjects {
		components := self.Components[k]
		if !isSubPath(root_components, components) {
			continue
		}

		walkFn(path_specs.NewSafeDatastorePath(components...))
	}

	return nil
}

func (self *TestDataStore) QueueMessageForClient(
	config_obj *config_proto.Config,
	client_id string,
	message *crypto_proto.VeloMessage) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	message.TaskId = self.idx + 1
	self.idx++

	result, pres := self.ClientTasks[client_id]
	if !pres {
		result = make([]*crypto_proto.VeloMessage, 0)
	}

	result = append(result, message)

	self.ClientTasks[client_id] = result
	return nil
}

func (self *TestDataStore) UnQueueMessageForClient(
	config_obj *config_proto.Config,
	client_id string,
	message *crypto_proto.VeloMessage) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	old_queue, pres := self.ClientTasks[client_id]
	if !pres {
		old_queue = make([]*crypto_proto.VeloMessage, 0)
	}

	new_queue := make([]*crypto_proto.VeloMessage, 0, len(old_queue))
	for _, item := range old_queue {
		if message.TaskId != item.TaskId {
			new_queue = append(new_queue, item)
		}
	}

	self.ClientTasks[client_id] = new_queue
	return nil
}

func (self *TestDataStore) Trace(name, filename string) {
	return
	fmt.Printf("Trace TestDataStore: %v: %v\n", name, filename)
}

func (self *TestDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	path := pathSpecToPath(urn, config_obj)
	self.Trace("GetSubject", path)
	result, pres := self.Subjects[path]
	if !pres {
		fallback_path := pathSpecToPath(
			urn.SetType(api.PATH_TYPE_DATASTORE_PROTO), config_obj)
		result, pres = self.Subjects[fallback_path]
		if !pres {
			return errors.WithMessage(os.ErrNotExist,
				fmt.Sprintf("While openning %v: not found", urn))
		}
	}
	proto.Merge(message, result)
	return nil
}

func (self *TestDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	filename := pathSpecToPath(urn, config_obj)
	self.Trace("SetSubject", filename)

	self.Subjects[filename] = message
	self.Components[filename] = urn.Components()

	return nil
}

func (self *TestDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	filename := pathSpecToPath(urn, config_obj)
	self.Trace("DeleteSubject", filename)
	delete(self.Subjects, filename)
	delete(self.Components, filename)

	return nil
}

// Lists all the children of a URN.
func (self *TestDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	offset uint64, length uint64) (
	[]api.DSPathSpec, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.Trace("ListChildren", pathDirSpecToPath(urn, config_obj))

	seen := make(map[string]bool)
	root_components := urn.Components()
	names := []string{}
	for _, components := range self.Components {
		// We only want direct children
		if len(components) != len(root_components)+1 {
			continue
		}

		if !isSubPath(root_components, components) {
			continue
		}

		name := components[len(components)-1]
		_, pres := seen[name]
		if !pres {
			names = append(names, name)
			seen[name] = true
		}
	}

	sort.Strings(names)

	result := make([]api.DSPathSpec, 0, len(names))
	for _, name := range names {
		result = append(result, urn.AddChild(name))
	}
	end := offset + length
	if end > uint64(len(result)) {
		end = uint64(len(result))
	}

	return result[offset:end], nil
}

// Update the posting list index. Searching for any of the
// keywords will return the entity urn.
func (self *TestDataStore) SetIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	entity = utils.SanitizeString(entity)

	for _, keyword := range keywords {
		keyword = utils.SanitizeString(strings.ToLower(keyword))
		components := index_urn.AddChild(keyword, entity)
		subject := pathSpecToPath(components, config_obj)
		self.Subjects[subject] = &empty.Empty{}
		self.Components[subject] = components.Components()
	}
	return nil
}

func (self *TestDataStore) UnsetIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	entity = utils.SanitizeString(entity)
	for _, keyword := range keywords {
		keyword = utils.SanitizeString(strings.ToLower(keyword))
		subject := pathSpecToPath(
			index_urn.AddChild(keyword, entity), config_obj)
		delete(self.Subjects, subject)
		delete(self.Components, subject)
	}
	return nil
}

func (self *TestDataStore) CheckIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	entity = utils.SanitizeString(entity)
	for _, keyword := range keywords {
		keyword = utils.SanitizeString(strings.ToLower(keyword))
		subject := pathSpecToPath(
			index_urn.AddChild(keyword, entity), config_obj)
		_, pres := self.Subjects[subject]
		if pres {
			return nil
		}
	}
	return errors.New("Client does not have label")
}

// List all direct children
func (self *TestDataStore) listChildren(urn api.DSPathSpec) []string {
	seen := make(map[string]bool)
	result := []string{}

	root_components := urn.Components()
	for _, components := range self.Components {
		if !isSubPath(root_components, components) {
			continue
		}

		if len(root_components) < len(components) {
			direct_child := components[len(root_components)]
			_, pres := seen[direct_child]
			if !pres {
				result = append(result, direct_child)
				seen[direct_child] = true
			}
		}
	}
	return result
}

func (self *TestDataStore) SearchClients(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	query string, query_type string,
	offset uint64, limit uint64, sort_direction SortingSense) []string {
	seen := make(map[string]bool)
	result := []string{}

	query = strings.ToLower(query)
	if query == "." || query == "" {
		query = "all"
	}

	add_func := func(key string) {
		children := self.listChildren(
			index_urn.AddChild(utils.SanitizeString(key)))
		for _, child_name := range children {
			name := utils.UnsanitizeComponent(child_name)
			_, pres := seen[name]
			if !pres {
				seen[name] = true

			}
		}
	}

	// Query has a wildcard.
	if strings.ContainsAny(query, "[]*?") {
		// We could make it smarter in future but this is
		// quick enough for now.
		for _, name := range self.listChildren(index_urn) {
			name = utils.UnsanitizeComponent(name)
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

func pathSpecToPath(p api.DSPathSpec,
	config_obj *config_proto.Config) string {
	result := p.AsDatastoreFilename(config_obj)

	// Sanitize it on windows to convert back to a common format
	// for comparisons.
	if runtime.GOOS == "windows" {
		return path.Clean(strings.Replace(strings.TrimPrefix(
			result, path_specs.WINDOWS_LFN_PREFIX), "\\", "/", -1))
	}

	return result
}

func pathDirSpecToPath(p api.DSPathSpec,
	config_obj *config_proto.Config) string {
	result := p.AsDatastoreDirectory(config_obj)

	// Sanitize it on windows to convert back to a common format
	// for comparisons.
	if runtime.GOOS == "windows" {
		return path.Clean(strings.Replace(strings.TrimPrefix(
			result, path_specs.WINDOWS_LFN_PREFIX), "\\", "/", -1))
	}

	return result
}
