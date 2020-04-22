package directory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/vfilter"
)

type DirectoryQueueManager struct {
	file_store api.FileStore
}

func (self *DirectoryQueueManager) Push(
	queue_name, source string, serialized_rows []byte) error {
	rows, err := parseJsonToDicts(serialized_rows)
	if err != nil {
		return err
	}

	_ = rows

	return nil
}

func (self *DirectoryQueueManager) Read(
	queue_name, source string, start_time, endtime time.Time) <-chan vfilter.Row {
	output := make(chan vfilter.Row)

	go func() {
		defer close(output)
	}()

	return output
}

func (self *DirectoryQueueManager) Watch(
	queue_name, source string) <-chan vfilter.Row {
	output := make(chan vfilter.Row)

	go func() {
		defer close(output)
	}()

	return output
}

func parseJsonToDicts(serialized []byte) ([]*ordereddict.Dict, error) {
	var raw_objects []json.RawMessage
	err := json.Unmarshal(serialized, &raw_objects)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result := make([]*ordereddict.Dict, 0, len(raw_objects))
	for _, raw_message := range raw_objects {
		item := ordereddict.NewDict()
		err = json.Unmarshal(raw_message, &item)
		if err != nil {
			continue
		}
		result = append(result, item)
	}

	return result, nil
}

// Figure out where we need to save the CSV file.
func GetCSVPath(queue_name, source string) string {
	artifact_name, artifact_source := QueryNameToArtifactAndSource(queue_name)
	day_name := paths.GetDayName()

	if artifact_source != "" {
		return fmt.Sprintf(
			"/journals/%s/%s/%s.csv",
			artifact_name, day_name, artifact_source)

	} else {
		return fmt.Sprintf(
			"/journals/%s/%s.csv",
			artifact_name, day_name)
	}

}

func QueryNameToArtifactAndSource(query_name string) (
	artifact_name, artifact_source string) {
	components := strings.Split(query_name, "/")
	switch len(components) {
	case 2:
		return components[0], components[1]
	default:
		return components[0], ""
	}
}

func NewDirectoryQueueManager(config_obj *config_proto.Config) *DirectoryQueueManager {
	return &DirectoryQueueManager{
		file_store: NewDirectoryFileStore(config_obj),
	}
}
