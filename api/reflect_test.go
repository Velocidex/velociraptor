package api

import (
	"fmt"
	"github.com/golang/protobuf/jsonpb"
	assert "github.com/stretchr/testify/assert"
	"testing"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
)

func TestDescriptor(t *testing.T) {
	result := &api_proto.Types{}
	seen := make(map[string]bool)
	add_type("proto.ApiClient", result, seen)

	marshaler := &jsonpb.Marshaler{Indent: " "}
	str, err := marshaler.MarshalToString(result)
	assert.NoError(t, err)

	fmt.Println(str)
}
