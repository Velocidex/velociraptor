package json

import (
	"bytes"
	"reflect"
	"sync"

	"github.com/Velocidex/json"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type RawMessage = json.RawMessage
type Marshaler = json.Marshaler

var (
	EncoderCallbackSkip = json.EncoderCallbackSkip

	bufferPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
)

func GetBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()

	return buf
}

func PutBuffer(buf *bytes.Buffer) {
	bufferPool.Put(buf)
}

func MarshalWithOptions(v interface{}, opts *json.EncOpts) ([]byte, error) {
	if opts == nil {
		return json.Marshal(v)
	}
	return json.MarshalWithOptions(v, opts)
}

func Marshal(v interface{}) ([]byte, error) {
	opts := DefaultEncOpts()
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

func MarshalString(v interface{}) (string, error) {
	result, err := Marshal(v)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func StringIndent(v interface{}) string {
	result, err := MarshalIndent(v)
	if err != nil {
		panic(err)
	}
	return string(result)
}

func MarshalIndent(v interface{}) ([]byte, error) {
	opts := DefaultEncOpts()
	return MarshalIndentWithOptions(v, opts)
}

func MarshalIndentWithOptions(v interface{}, opts *json.EncOpts) ([]byte, error) {
	b, err := json.MarshalWithOptions(v, opts)
	if err != nil {
		return nil, err
	}

	buf := GetBuffer()
	defer PutBuffer(buf)
	err = json.Indent(buf, b, "", " ")
	if err != nil {
		return nil, err
	}

	// Need to make a copy because the real buffer will be reused in
	// the pool.
	return CopySlice(buf.Bytes()), nil
}

func MarshalJsonl(v interface{}) ([]byte, error) {
	rt := reflect.TypeOf(v)
	if rt == nil || rt.Kind() != reflect.Slice && rt.Kind() != reflect.Array {
		return nil, json.EncoderCallbackSkip
	}

	a_slice := reflect.ValueOf(v)

	options := DefaultEncOpts()

	out := GetBuffer()
	defer PutBuffer(out)

	for i := 0; i < a_slice.Len(); i++ {
		row := a_slice.Index(i).Interface()
		serialized, err := json.MarshalWithOptions(row, options)
		if err != nil {
			return nil, err
		}
		out.Write(serialized)
		out.Write([]byte{'\n'})
	}

	// Need to make a copy because the real buffer will be reused in
	// the pool.
	return CopySlice(out.Bytes()), nil
}

func Unmarshal(b []byte, v interface{}) error {

	self, ok := v.(proto.Message)
	if ok {
		options := &protojson.UnmarshalOptions{
			DiscardUnknown: true,
		}
		return options.Unmarshal(b, self)
	}

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

func CopySlice(in []byte) []byte {
	result := make([]byte, len(in))
	copy(result, in)
	return result
}

func Indent(dst *bytes.Buffer, src []byte, prefix, indent string) error {
	return json.Indent(dst, src, prefix, indent)
}
