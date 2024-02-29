package file_store

import (
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

// Flush all the filestores if needed. Not all filestore
// implementations need to be flushed, so this function will retun
// immediately if not required. If the filestore does need to be
// flushed this operation may be expensive so it should only be done
// when it is important to have data immediately visible.
func FlushFilestore(config_obj *config_proto.Config) error {
	var wg sync.WaitGroup
	defer wg.Wait()

	file_store_factory := GetFileStore(config_obj)
	flusher, ok := file_store_factory.(api.Flusher)

	if ok {
		wg.Add(1)
		go func() {
			defer wg.Done()
			flusher.Flush()
		}()
	}

	return nil
}
