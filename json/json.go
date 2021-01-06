// Wrap json library to control encoding.

package json

import (
	"github.com/Velocidex/json"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
)

func MarshalJSONDict(v interface{}, opts *json.EncOpts) ([]byte, error) {
	self, ok := v.(*ordereddict.Dict)
	if !ok || self == nil {
		return nil, json.EncoderCallbackSkip
	}

	result := "{"
	for _, k := range self.Keys() {

		// add key
		kEscaped, err := json.MarshalWithOptions(k, opts)
		if err != nil {
			continue
		}

		result += string(kEscaped) + ":"

		// add value
		v, ok := self.Get(k)
		if !ok {
			v = "null"
		}

		// If v is a callable, run it
		callable, ok := v.(func() vfilter.Any)
		if ok {
			v = callable()
		}

		vBytes, err := json.MarshalWithOptions(v, opts)
		if err == nil {
			result += string(vBytes) + ","
		} else {
			result += "null,"
		}
	}
	if len(self.Keys()) > 0 {
		result = result[0 : len(result)-1]
	}
	result = result + "}"
	return []byte(result), nil
}

func init() {
	RegisterCustomEncoder(ordereddict.NewDict(), MarshalJSONDict)
}
