package api

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/jsonpb"
	assert "github.com/stretchr/testify/assert"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
)

func TestDescriptor(t *testing.T) {
	result := &artifacts_proto.Types{}
	seen := make(map[string]bool)
	add_type("proto.ApiClient", result, seen)

	marshaler := &jsonpb.Marshaler{Indent: " "}
	str, err := marshaler.MarshalToString(result)
	assert.NoError(t, err)

	fmt.Println(str)
}
