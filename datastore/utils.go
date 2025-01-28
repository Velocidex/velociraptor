package datastore

import (
	"errors"
	"sync"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	WalkWithDirectories    = true
	WalkWithoutDirectories = false
)

type MultiGetSubjectRequest struct {
	mu      sync.Mutex
	message proto.Message

	Path api.DSPathSpec
	Err  error

	// Free form data that goes with the request.
	Data interface{}
}

// Return a copy so there is no race
func (self *MultiGetSubjectRequest) Message() proto.Message {
	return proto.Clone(self.message)
}

func (self *MultiGetSubjectRequest) Error() error {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.Err
}

func NewMultiGetSubjectRequest(message proto.Message, path api.DSPathSpec, data interface{}) *MultiGetSubjectRequest {
	return &MultiGetSubjectRequest{
		message: proto.Clone(message),
		Path:    path,
		Data:    data,
	}
}

// A helper function to read multipe subjects at the same time.
func MultiGetSubject(
	config_obj *config_proto.Config,
	requests []*MultiGetSubjectRequest) error {

	db, err := GetDB(config_obj)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, request := range requests {
		wg.Add(1)

		go func(request *MultiGetSubjectRequest) {
			defer wg.Done()

			request.mu.Lock()
			defer request.mu.Unlock()
			request.Err = db.GetSubject(config_obj, request.Path, request.message)
		}(request)
	}

	wg.Wait()
	return nil
}

func Walk(config_obj *config_proto.Config,
	datastore DataStore, root api.DSPathSpec,
	with_directories bool,
	walkFn WalkFunc) error {

	TraceDirectory(datastore, config_obj, "Walk", root)
	all_children, err := datastore.ListChildren(config_obj, root)
	if err != nil {
		return err
	}

	directories := []api.DSPathSpec{}
	files := []api.DSPathSpec{}

	for _, child := range all_children {
		// Recurse into directories
		if child.IsDir() {
			directories = append(directories, child)
		} else {
			files = append(files, child)
		}
	}

	// Depth first walk - first directories then files. This allows us
	// to remove empty directories recursively.
	for _, d := range directories {
		err := Walk(config_obj, datastore, d, with_directories, walkFn)
		if err != nil {
			// Do not quit the walk early.
		}
	}

	if with_directories {
		for _, d := range directories {
			err := walkFn(d)
			if err == StopIteration {
				return nil
			}
		}
	}

	for _, f := range files {
		err := walkFn(f)
		if err == StopIteration {
			return nil
		}
	}

	return nil
}

func RecursiveDelete(
	config_obj *config_proto.Config,
	datastore DataStore, root api.DSPathSpec) error {
	return Walk(config_obj, datastore, root, false,
		func(urn api.DSPathSpec) error {
			// Ignore errors so we can keep going as much as possible.
			_ = datastore.DeleteSubjectWithCompletion(
				config_obj, urn, utils.BackgroundWriter)
			return nil
		})
}

func GetImplementationName(
	config_obj *config_proto.Config) (string, error) {
	if config_obj.Datastore == nil {
		return "", errors.New("Invalid datastore config")
	}

	if config_obj.Frontend == nil {
		return config_obj.Datastore.Implementation, nil
	}

	if config_obj.Frontend.IsMinion &&
		config_obj.Datastore.MinionImplementation != "" {
		return config_obj.Datastore.MinionImplementation, nil
	}

	if config_obj.Datastore.MasterImplementation != "" {
		return config_obj.Datastore.MasterImplementation, nil
	}

	return config_obj.Datastore.Implementation, nil
}

type Flusher interface {
	Flush()
}

func FlushDatastore(config_obj *config_proto.Config) error {
	var wg sync.WaitGroup
	defer wg.Wait()

	db, err := GetDB(config_obj)
	if err != nil {
		return err
	}

	flusher, ok := db.(Flusher)
	if ok {
		wg.Add(1)
		go func() {
			defer wg.Done()
			flusher.Flush()
		}()
	}

	return nil
}
