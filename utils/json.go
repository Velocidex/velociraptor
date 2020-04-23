package utils

import (
	"encoding/json"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
)

func ParseJsonToDicts(serialized []byte) ([]*ordereddict.Dict, error) {
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
