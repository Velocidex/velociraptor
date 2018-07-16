package actions

import (
	"github.com/dustin/go-humanize"
	"log"
	"strings"
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
	for query_idx, query := range arg.Query {
		now := uint64(time.Now().UTC().UnixNano() / 1000)
		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			responder.RaiseError(err.Error())
			return
		}

		// Use defaults for MaxRow if not defined.
		max_rows := int(arg.MaxRow)
		if max_rows == 0 {
			max_rows = 10000
		}
		result_chan := vfilter.GetResponseChannel(
			vql, ctx, scope, max_rows)
		for {
			result, ok := <-result_chan
			if !ok {
				break
			}

			// Skip let queries since they never produce results.
			if strings.HasPrefix(strings.ToLower(query.VQL), "let") {
				continue
			}
			response := &actions_proto.VQLResponse{
				Query:     query,
				QueryId:   uint64(query_idx),
				Part:      uint64(result.Part),
				Response:  string(result.Payload),
				Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
			}

			responder.Log("Time %v: %s: Sending response part %d %s bytes (%d rows).",
				(response.Timestamp-now)/1000000,
				query.Name,
				result.Part,
				humanize.Bytes(uint64(len(result.Payload))),
				result.TotalRows,
			)

			response.Columns = *vql.Columns(scope)
			responder.AddResponse(response)
		}
	}

	if uploader.Count > 0 {
		responder.Log("Uploaded %v files.", uploader.Count)
	}

	responder.Return()
}
