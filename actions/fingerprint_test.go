package actions_test

import (
	"github.com/golang/protobuf/proto"
	assert "github.com/stretchr/testify/assert"
	"os"
	"testing"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	"www.velocidex.com/golang/velociraptor/responder"
)

func TestHashFile(t *testing.T) {
	cwd, _ := os.Getwd()
	pathspec := &actions_proto.PathSpec{
		Path:     proto.String(cwd),
		Pathtype: actions_proto.PathSpec_OS.Enum(),
		NestedPath: &actions_proto.PathSpec{
			Path:     proto.String("test_data"),
			Pathtype: actions_proto.PathSpec_OS.Enum(),
			NestedPath: &actions_proto.PathSpec{
				Path:     proto.String("hello.txt"),
				Pathtype: actions_proto.PathSpec_OS.Enum(),
			},
		},
	}
	ctx := context.Background()
	tuple := &actions_proto.FingerprintTuple{
		FpType: actions_proto.FingerprintTuple_FPT_GENERIC.Enum(),
		Hashers: []actions_proto.FingerprintTuple_HashType{
			*actions_proto.FingerprintTuple_SHA1.Enum(),
			*actions_proto.FingerprintTuple_SHA256.Enum(),
		},
	}
	request := actions_proto.FingerprintRequest{
		Pathspec: pathspec,
		Tuples: []*actions_proto.FingerprintTuple{
			tuple,
		},
	}

	hash_file := actions.HashFile{}
	arg, err := responder.NewRequest(&request)
	if err != nil {
		t.Fatal(err)
	}
	responses := GetResponsesFromAction(&hash_file, &ctx, arg)
	assert.Equal(t, len(responses), 2)
	response := responder.ExtractGrrMessagePayload(
		responses[0]).(*actions_proto.FingerprintResponse)

	assert.Equal(t, response.Hash.Sha256,
		[]uint8([]byte{0xf5, 0x44, 0xea, 0x51, 0xaa, 0x45, 0x9, 0x89,
			0x7, 0x1e, 0xc7, 0x95, 0xc1, 0xad, 0x45, 0x36, 0xdb,
			0xb3, 0x7b, 0x9c, 0xcd, 0x22, 0xec, 0xaa, 0x39, 0x92,
			0x33, 0xdb, 0xc1, 0x9d, 0xdb, 0xc0}))
}
