package datastore

import (
	"errors"
	"sync"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

const (
	WalkWithDirectories    = true
	WalkWithoutDirectories = false
)

type MultiGetSubjectRequest struct {
	Path    api.DSPathSpec
	Message proto.Message
	Err     error

	// Free form data that goes with the request.
	Data interface{}
}

// A helper function to read multipe subjects at the same time.
func MultiGetSubject(
	config_obj *config_proto.Config,
	requests []*MultiGetSubjectRequest) error {

	var mu sync.Mutex

	db, err := GetDB(config_obj)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	mu.Lock()
	for _, request := range requests {
		wg.Add(1)
		go func(request *MultiGetSubjectRequest) {
			mu.Lock()
			defer mu.Unlock()
			request.Err = db.GetSubject(config_obj, request.Path, request.Message)
			wg.Done()
		}(request)
	}
	mu.Unlock()

	wg.Wait()
	return nil
}

func Walk(config_obj *config_proto.Config,
	datastore DataStore, root api.DSPathSpec,
	with_directories bool,
	walkFn WalkFunc) error {

	TraceDirectory(config_obj, "Walk", root)
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
