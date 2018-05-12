package actions

import (
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
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := NewResponder(msg, output)
	arg, pres := responder.GetArgs().(*actions_proto.VQLCollectorArgs)
	if !pres {
		responder.RaiseError("Request should be of type VQLCollectorArgs")
		return
	}

	if arg.Query == nil {
		responder.RaiseError("Query should be specified.")
		return
	}

	scope := vql_subsystem.MakeScope()

	// All the queries will use the same scope. This allows one
	// query to define functions for the next query in order.
	for _, query := range arg.Query {
		vql, err := vfilter.Parse(query)
		if err != nil {
			responder.RaiseError(err.Error())
			return
		}

		s, err := vfilter.OutputJSON(vql, ctx, scope)
		if err != nil {
			responder.RaiseError(err.Error())
			return
		}

		response := &actions_proto.VQLResponse{
			Response: proto.String(string(s)),
		}

		columns := vql.Columns(scope)
		for _, column := range *columns {
			response.Columns = append(response.Columns, column)
		}

		vfilter.Debug(response)
		responder.AddResponse(response)
	}

	responder.Return()
}
