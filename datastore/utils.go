package datastore

import (
	"errors"
	"sync"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
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
	datastore DataStore, root api.DSPathSpec, walkFn WalkFunc) error {

	TraceDirectory(config_obj, "Walk", root)
	all_children, err := datastore.ListChildren(config_obj, root)
	if err != nil {
		return err
	}

	for _, child := range all_children {
		// Recurse into directories
		if child.IsDir() {
			err := Walk(config_obj, datastore, child, walkFn)
			if err != nil {
				// Do not quit the walk early.
			}

		} else {
			err := walkFn(child)
			if err == StopIteration {
				return nil
			}
			continue
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
