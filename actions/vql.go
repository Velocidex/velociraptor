package actions

import (
	"encoding/json"
	"github.com/golang/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type VQLClientAction struct{}

func (self *VQLClientAction) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	responder := NewResponder(msg)
	arg, pres := responder.GetArgs().(*actions_proto.VQLCollectorArgs)
	if !pres {
		return responder.RaiseError("Request should be of type VQLCollectorArgs")
	}

	if arg.Query == nil {
		return responder.RaiseError("Query should be specified.")
	}

	vql, err := vfilter.Parse(*arg.Query)
	if err != nil {
		responder.RaiseError(err.Error())
	}

	scope := vql_subsystem.MakeScope()
	output_chan := vql.Eval(ctx, scope)
	result := []vfilter.Row{}
	for row := range output_chan {
		result = append(result, row)
	}

	s, err := json.MarshalIndent(result, "", " ")
	if err != nil {
		return responder.RaiseError(err.Error())
	}

	responder.AddResponse(&actions_proto.VQLResponse{
		Response: proto.String(string(s)),
	})

	return responder.Return()
}
