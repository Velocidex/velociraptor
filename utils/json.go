package utils

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
)

func ParseJsonToDicts(serialized []byte) ([]*ordereddict.Dict, error) {
	if len(serialized) == 0 {
		return nil, nil
	}

	// Support decoding an array of objects.
	if serialized[0] == '[' {
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
				return nil, err
			}
			result = append(result, item)
		}

		return result, nil
	}

	// Otherwise, it must be JSONL
	lines := bytes.Split(serialized, []byte{'\n'})
	result := make([]*ordereddict.Dict, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		item := ordereddict.NewDict()
		err := json.Unmarshal(line, &item)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}

	return result, nil
}

func DictsToJson(rows []*ordereddict.Dict) ([]byte, error) {
	out := bytes.Buffer{}
	for _, row := range rows {
		serialized, err := json.Marshal(row)
		if err != nil {
			return nil, err
		}

		out.Write(serialized)
		out.Write([]byte{'\n'})
	}

	return out.Bytes(), nil
}

// Convert old json format to jsonl.
func JsonToJsonl(rows []byte) ([]byte, error) {
	if len(rows) == 0 {
		return rows, nil
	}

	// I am tempted to store the json directly in the database
	// avoiding the roundtrip but this means that it might be
	// possible to inject invalid json to the database. For now we
	// take the performance hit and then think of something
	// better.
	dict_rows, err := ParseJsonToDicts(rows)
	if err != nil {
		return nil, err
	}
	return DictsToJson(dict_rows)
}

func ReadJsonFromFile(ctx context.Context, fd io.Reader) chan *ordereddict.Dict {
	output_chan := make(chan *ordereddict.Dict)

	go func() {
		defer close(output_chan)

		reader := bufio.NewReader(fd)

		for {
			select {
			case <-ctx.Done():
				return

			default:
				row_data, err := reader.ReadBytes('\n')
				if len(row_data) == 0 || err != nil {
					return
				}
				item := ordereddict.NewDict()
				err = item.UnmarshalJSON(row_data)
				if err != nil {
					continue
				}

				output_chan <- item
			}
		}
	}()

	return output_chan
}
