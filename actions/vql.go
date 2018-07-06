package actions

import (
	"log"
	"time"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type LogWriter struct {
	responder *responder.Responder
}

func (self *LogWriter) Write(b []byte) (int, error) {
	err := self.responder.Log("%s", string(b))
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

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
	uploader := &vql_subsystem.VelociraptorUploader{
		Responder: responder,
	}
	env := vfilter.NewDict().
		Set("$responder", responder).
		Set("$uploader", uploader).
		Set("config", ctx.Config)

	for _, env_spec := range arg.Env {
		env.Set(env_spec.Key, env_spec.Value)
	}

	scope := vql_subsystem.MakeScope().AppendVars(env)
	scope.Logger = log.New(&LogWriter{responder},
		"vql: ", log.Lshortfile)

	// All the queries will use the same scope. This allows one
	// query to define functions for the next query in order.
	for _, query := range arg.Query {
		now := uint64(time.Now().UTC().UnixNano() / 1000)
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

		responder.Log("Ran query %s in %v seconds.", query.Name,
			(response.Timestamp-now)/1000000)

		response.Columns = *vql.Columns(scope)
		responder.AddResponse(response)
	}

	if uploader.Count > 0 {
		responder.Log("Uploaded %v files.", uploader.Count)
	}

	responder.Return()
}
