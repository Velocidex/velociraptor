// Wrap json library to control encoding.

package json

import (
	"bytes"
	"context"
	"reflect"

	"github.com/Velocidex/json"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/protocols"
	"www.velocidex.com/golang/vfilter/types"
)

// Delay instantiation until all registrations occured
func DefaultEncOpts() *json.EncOpts {
	// Need to return a fresh object each time because json.encode
	// modifies the opts object.
	return NewEncOpts()
}

type EncOpts = json.EncOpts

func MarshalJSONDict(v interface{}, opts *json.EncOpts) ([]byte, error) {
	if v == nil || (reflect.ValueOf(v).Kind() == reflect.Ptr &&
		reflect.ValueOf(v).IsNil()) {
		return []byte("{}"), nil
	}

	self, ok := v.(*ordereddict.Dict)
	if !ok || self == nil {
		return nil, json.EncoderCallbackSkip
	}

	buf := &bytes.Buffer{}

	buf.Write([]byte("{"))
	for _, i := range self.Items() {

		// add key
		kEscaped, err := json.MarshalWithOptions(i.Key, opts)
		if err != nil {
			continue
		}

		buf.Write(kEscaped)
		buf.Write([]byte(":"))

		if i.Value == nil {
			i.Value = &vfilter.Null{}
		}

		// If v is a callable, run it
		callable, ok := i.Value.(func() vfilter.Any)
		if ok {
			i.Value = callable()
		}

		vBytes, err := json.MarshalWithOptions(i.Value, opts)
		if err == nil {
			buf.Write(vBytes)
			buf.Write([]byte(","))
		} else {
			buf.Write([]byte("null,"))
		}
	}

	if self.Len() > 0 {
		buf.Truncate(buf.Len() - 1)
	}
	buf.Write([]byte("}"))
	return buf.Bytes(), nil
}

func MarshalLazyFunctions(v interface{}, opts *json.EncOpts) ([]byte, error) {
	lazy_expr, ok := v.(types.LazyExpr)
	if ok {
		return json.MarshalWithOptions(
			lazy_expr.Reduce(context.Background()), opts)
	}
	return nil, json.EncoderCallbackSkip
}

func init() {
	RegisterCustomEncoder(ordereddict.NewDict(), MarshalJSONDict)
	RegisterCustomEncoder(&protocols.LazyFunctionWrapper{}, MarshalLazyFunctions)
}
