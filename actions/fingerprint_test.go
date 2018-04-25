package actions

import (
	"os"
	"testing"
	_ "github.com/stretchr/testify/assert"
	"github.com/golang/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/context"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	utils "www.velocidex.com/golang/velociraptor/testing"
)


func TestHashFile(t *testing.T) {
	cwd, _ := os.Getwd()
	pathspec := &actions_proto.PathSpec{
		Path: proto.String(cwd),
		Pathtype: actions_proto.PathSpec_OS.Enum(),
		NestedPath: &actions_proto.PathSpec{
			Path: proto.String("test_data"),
			Pathtype: actions_proto.PathSpec_OS.Enum(),
			NestedPath: &actions_proto.PathSpec{
				Path: proto.String("hello.txt"),
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

	hash_file := HashFile{}
	arg, err := NewRequest(&request)
	if err != nil {
		t.Fatal(err)
	}
	responses := hash_file.Run(&ctx, arg)
	utils.Debug(responses)
}
