package utils

import (
	"encoding/json"

	"github.com/Velocidex/ordereddict"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/vfilter"
)

/*
   Collection requests may specify a spec. Since multiple artifacts
   can be collected in the same flow, the spec is a way to specify
   which parameters go with which artifact.

   This is what a spec looks like.

   ```json
   {
     "Artifact.Name": {
        "Parameter1": "Value1",
        "Parameter2": "Value2"
      }
   }
   ```

   It is a dict with keys being the artifact name and values are
   parameters for each artifact.

   This format of a spec is easier for people to use in VQL and it is
   very simple. However, the actual client request is more
   complicated. It consists of the []flows_proto.ArtifacSpec protobuf:

   ```json
   [{
     "artifact": "Artifact.Name",
     "parameters": {
        "env": [{
           "key": "Parameter1",
           "value: "Value1"
        } , {
           "key": "Parameter2",
           "value: "Value2"
        }],
      }
   }]
   ```

   Plugins which accept a spec need to handle both forms - sometimes
   the protobuf form is emitted into the query (e.g. the output of
   flows() or hunts() ) and this may be used as an input to
   e.g. collect_client() or hunt() )

   These functions allow converting from one form to the other.
*/

// Convert the spec which could be a protobuf to the standardized Dict
// specs.
func ConvertToArtifactSpecs(spec vfilter.Any) (res *ordereddict.Dict, err error) {
	var serialized []byte

	switch t := spec.(type) {
	case *ordereddict.Dict:
		return t, nil
	case string:
		// It could be a serialized dict or serialized protobuf
		res = ordereddict.NewDict()
		err = json.Unmarshal([]byte(t), res)
		if err == nil {
			_, pres := res.GetString("artifact")
			if !pres {
				return res, nil
			}
		}

		serialized = []byte(t)
	default:
		serialized, err = json.Marshal(spec)
		if err != nil {
			return nil, err
		}
	}

	// Maybe multiple specs
	specs := []*flows_proto.ArtifactSpec{}
	err = json.Unmarshal(serialized, &specs)
	if err != nil {
		// Or just one
		spec := &flows_proto.ArtifactSpec{}
		err = json.Unmarshal(serialized, spec)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}

	spec_dict := ordereddict.NewDict()
	for _, spec := range specs {
		env := ordereddict.NewDict()
		if spec.Parameters != nil {
			for _, item := range spec.Parameters.Env {
				env.Set(item.Key, item.Value)
			}
		}
		spec_dict.Set(spec.Artifact, env)
	}
	return spec_dict, nil
}
