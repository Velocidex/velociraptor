package json

import (
	"github.com/Velocidex/json"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
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

func init() {
	RegisterCustomEncoder(&flows_proto.ArtifactCollectorContext{},
		MarshalHuntProtobuf)
}
