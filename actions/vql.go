package actions

import (
	"log"
	"os"
	"time"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type VQLClientAction struct{}

func (self *VQLClientAction) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(msg, output)
	arg, pres := responder.GetArgs().(*actions_proto.VQLCollectorArgs)
	if !pres {
		responder.RaiseError("Request should be of type VQLCollectorArgs")
		return
	}

	if arg.Query == nil {
		responder.RaiseError("Query should be specified.")
		return
	}

	// Create a new query environment and store some useful
	// objects in there. VQL plugins may then use the environment
	// to communicate with the server.
	env := vfilter.NewDict().
		Set("$responder", responder).
		Set("$uploader", &vql_subsystem.VelociraptorUploader{responder}).
		Set("config", ctx.Config)

	for _, env_spec := range arg.Env {
		env.Set(env_spec.Key, env_spec.Value)
	}

	scope := vql_subsystem.MakeScope().AppendVars(env)
	scope.Logger = log.New(os.Stderr, "vql: ", log.Lshortfile)

	// All the queries will use the same scope. This allows one
	// query to define functions for the next query in order.
	for _, query := range arg.Query {
		vql, err := vfilter.Parse(query.VQL)
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
			Query:     query,
			Response:  string(s),
			Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
		}

		columns := vql.Columns(scope)
		for _, column := range *columns {
			response.Columns = append(response.Columns, column)
		}

		responder.AddResponse(response)
	}

	responder.Return()
}
