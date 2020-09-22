package services

import (
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Helpers for testing the filestore.

// Read num_rows messages from the filestore queues and fill in the
// result array.
func GetPublishedEvents(
	config_obj *config_proto.Config,
	artifact string,
	wg *sync.WaitGroup,
	num_rows int,
	result *[]*ordereddict.Dict) {

	local_wg := &sync.WaitGroup{}
	local_wg.Add(1)

	go func() {
		defer wg.Done()

		journal, err := GetJournal()
		if err != nil {
			return
		}
		events, cancel := journal.Watch(artifact)
		defer cancel()

		// Wait here until we are set up.
		local_wg.Done()

		for row := range events {
			*result = append(*result, row)
			if len(*result) == num_rows {
				return
			}
		}
	}()

	local_wg.Wait()
}
