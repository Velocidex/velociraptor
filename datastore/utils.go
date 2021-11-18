package datastore

import (
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

	db, err := GetDB(config_obj)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, request := range requests {
		wg.Add(1)
		go func() {
			request.Err = db.GetSubject(config_obj, request.Path, request.Message)
			wg.Done()
		}()
	}

	wg.Wait()
	return nil
}
