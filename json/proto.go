package json

import (
	"github.com/Velocidex/json"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Register a custom encoder for protobufs since they have some weird fields.
func MarshalHuntProtobuf(v interface{}, opts *EncOpts) ([]byte, error) {
	message, ok := v.(proto.Message)
	if ok {
		options := protojson.MarshalOptions{
			UseProtoNames:   true,
			UseEnumNumbers:  false,
			EmitUnpopulated: false,
		}
		return options.Marshal(message)
	}
	return nil, json.EncoderCallbackSkip
}
