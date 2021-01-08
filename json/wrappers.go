package json

import (
	"bytes"
	"reflect"

	"github.com/Velocidex/json"
)

func MarshalWithOptions(v interface{}, opts *json.EncOpts) ([]byte, error) {
	if opts == nil {
		return json.Marshal(v)
	}
	return json.MarshalWithOptions(v, opts)
}

func Marshal(v interface{}) ([]byte, error) {
	opts := NewEncOpts()
	return json.MarshalWithOptions(v, opts)
}

func MustMarshalIndent(v interface{}) []byte {
	result, err := MarshalIndent(v)
	if err != nil {
		panic(err)
	}
	return result
}

func MustMarshalString(v interface{}) string {
	result, err := Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(result)
}

func StringIndent(v interface{}) string {
	result, err := MarshalIndent(v)
	if err != nil {
		panic(err)
	}
	return string(result)
}

func MarshalIndent(v interface{}) ([]byte, error) {
	opts := NewEncOpts()
	return MarshalIndentWithOptions(v, opts)
}

func MarshalIndentWithOptions(v interface{}, opts *json.EncOpts) ([]byte, error) {
	b, err := json.MarshalWithOptions(v, opts)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = json.Indent(&buf, b, "", " ")
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func MarshalJsonl(v interface{}) ([]byte, error) {
	rt := reflect.TypeOf(v)
	if rt == nil || rt.Kind() != reflect.Slice && rt.Kind() != reflect.Array {
		return nil, json.EncoderCallbackSkip
	}

	a_slice := reflect.ValueOf(v)

	options := NewEncOpts()
	out := bytes.Buffer{}
	for i := 0; i < a_slice.Len(); i++ {
		row := a_slice.Index(i).Interface()
		serialized, err := json.MarshalWithOptions(row, options)
		if err != nil {
			return nil, err
		}
		out.Write(serialized)
		out.Write([]byte{'\n'})
	}
	return out.Bytes(), nil
}

func Unmarshal(b []byte, v interface{}) error {
	return json.Unmarshal(b, v)
}

// Marshals into a normalized string with sorted keys - this is most
// important for tests.
func MarshalIndentNormalized(v interface{}) ([]byte, error) {
	serialized, err := Marshal(v)
	if err != nil {
		return nil, err
	}

	data := make(map[string]interface{})
	err = Unmarshal(serialized, &data)
	if err != nil {
		return nil, err
	}

	return MarshalIndent(data)
}
