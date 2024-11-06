package utils

import (
	"bufio"
	"bytes"
	"context"
	"io"

	"github.com/Velocidex/json"
	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"

	vjson "www.velocidex.com/golang/velociraptor/json"
)

func ParseJsonToObject(serialized []byte) (*ordereddict.Dict, error) {
	if len(serialized) == 0 || serialized[0] != '{' {
		return nil, errors.New("Invalid JSON object")
	}

	item := ordereddict.NewDict()
	err := json.Unmarshal(serialized, &item)
	return item, err
}

func ParseJsonToDicts(serialized []byte) ([]*ordereddict.Dict, error) {
	if len(serialized) == 0 {
		return nil, nil
	}

	// Support decoding an array of objects.
	if serialized[0] == '[' {
		var raw_objects []json.RawMessage
		err := json.Unmarshal(serialized, &raw_objects)
		if err != nil {
			return nil, errors.Wrap(err, 0)
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

func DictsToJson(rows []*ordereddict.Dict, opts *json.EncOpts) ([]byte, error) {
	out := bytes.Buffer{}
	for _, row := range rows {
		serialized, err := vjson.MarshalWithOptions(row, opts)
		if err != nil {
			return nil, err
		}

		out.Write(serialized)
		out.Write([]byte{'\n'})
	}

	return out.Bytes(), nil
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
				item, err := ParseJsonToObject(row_data)
				if err != nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case output_chan <- item:
				}
			}
		}
	}()

	return output_chan
}
