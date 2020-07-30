package json

import (
	"bytes"

	"github.com/Velocidex/json"
	"www.velocidex.com/golang/vfilter"
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
	rows, ok := v.([]vfilter.Row)
	if !ok {
		return nil, json.EncoderCallbackSkip
	}

	options := NewEncOpts()
	out := bytes.Buffer{}
	for _, row := range rows {
		serialized, err := json.MarshalWithOptions(row, options)
		if err != nil {
			return nil, err
		}
		out.Write(serialized)
	}
	return out.Bytes(), nil
}

func Unmarshal(b []byte, v interface{}) error {
	return json.Unmarshal(b, v)
}
