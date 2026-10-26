package utils

import (
	"testing"

	"github.com/Velocidex/ordereddict"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"
)

func TestConvertSpecs(t *testing.T) {
	var golden []*ordereddict.Dict
	for _, tc := range []struct {
		name string
		item vfilter.Any
	}{{
		name: "Dict spec",
		item: ordereddict.NewDict().Set("Artifact1",
			ordereddict.NewDict().Set("P1", "V1")),
	}, {
		name: "Spec as a protobuf",
		item: []*flows_proto.ArtifactSpec{{
			Artifact: "Artifact1",
			Parameters: &flows_proto.ArtifactParameters{
				Env: []*actions_proto.VQLEnv{{
					Key:   "P1",
					Value: "V1",
				}}},
		}},
	}, {
		name: "Spec as a protobuf converted to dicts",
		item: []*ordereddict.Dict{
			ordereddict.NewDict().
				Set("artifact", "Artifact1").
				Set("parameters", ordereddict.NewDict().
					Set("env", []*ordereddict.Dict{
						ordereddict.NewDict().
							Set("key", "P1").
							Set("value", "V1"),
					})),
		},
	}, {
		name: "Spec as a json string",
		item: `{"Artifact1": {"P1": "V1"}}`,
	}, {
		name: "Spec as a json array of protobuf",
		item: `[{"artifact": "Artifact1", "parameters": {"env": [{"key": "P1","value":"V1"}]}}]`,
	}} {
		spec, err := ConvertToArtifactSpecs(tc.item)
		err_str := ""
		if err != nil {
			err_str = err.Error()
		}
		golden = append(golden, ordereddict.NewDict().
			Set("name", tc.name).
			Set("spec", spec).
			Set("err", err_str))
	}

	goldie.Assert(t, "TestConvertSpecs", json.MustMarshalIndent(golden))
}
