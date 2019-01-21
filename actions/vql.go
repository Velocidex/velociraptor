package actions

import (
	"context"
	"log"
	"runtime/debug"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_networking "www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
)

type LogWriter struct {
	config_obj *api_proto.Config
	responder  *responder.Responder
}

func (self *LogWriter) Write(b []byte) (int, error) {
	logging.GetLogger(self.config_obj, &logging.FrontendComponent).Info(string(b))
	err := self.responder.Log("%s", string(b))
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

type VQLClientAction struct{}

func (self *VQLClientAction) Run(
	config_obj *api_proto.Config,
	ctx context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(msg, output)
	arg, pres := responder.GetArgs().(*actions_proto.VQLCollectorArgs)
	if !pres {
		responder.RaiseError("Request should be of type VQLCollectorArgs")
		return
	}

	self.StartQuery(config_obj, ctx, responder, arg)
}

func (self *VQLClientAction) StartQuery(
	config_obj *api_proto.Config,
	ctx context.Context,
	responder *responder.Responder,
	arg *actions_proto.VQLCollectorArgs) {

	// Set reasonable defaults.
	if arg.MaxWait == 0 {
		arg.MaxWait = config_obj.Client.DefaultMaxWait

		if arg.MaxWait == 0 {
			arg.MaxWait = 100
		}
	}

	if arg.MaxRow == 0 {
		arg.MaxRow = 10000
	}

	rate := arg.OpsPerSecond
	if rate == 0 {
		rate = 1000000
	}

	if arg.Query == nil {
		responder.RaiseError("Query should be specified.")
		return
	}

	// If we panic we need to recover and report this to the
	// server.
	defer func() {
		if r := recover(); r != nil {
			responder.RaiseError(string(debug.Stack()))
		}
	}()

	// Create a new query environment and store some useful
	// objects in there. VQL plugins may then use the environment
	// to communicate with the server.
	uploader := &vql_networking.VelociraptorUploader{
		Responder: responder,
	}

	env := vfilter.NewDict().
		Set("$responder", responder).
		Set("$uploader", uploader).
		Set("config", config_obj).
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	for _, env_spec := range arg.Env {
		env.Set(env_spec.Key, env_spec.Value)
	}

	// Clients do not have a copy of artifacts so they need to be
	// sent all artifacts from the server.
	repository := artifacts.NewRepository()
	for _, artifact := range arg.Artifacts {
		repository.Set(artifact)
	}
	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = log.New(&LogWriter{config_obj, responder},
		"vql: ", log.Lshortfile)

	vfilter.InstallThrottler(scope, vfilter.NewTimeThrottler(float64(rate)))

	// All the queries will use the same scope. This allows one
	// query to define functions for the next query in order.
	for query_idx, query := range arg.Query {
		query_start := uint64(time.Now().UTC().UnixNano() / 1000)
		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			responder.RaiseError(err.Error())
			return
		}

		result_chan := vfilter.GetResponseChannel(
			vql, ctx, scope, int(arg.MaxRow), int(arg.MaxWait))
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
			// Don't log empty VQL statements.
			if query.Name != "" {
				responder.Log(
					"Time %v: %s: Sending response part %d %s (%d rows).",
					(response.Timestamp-query_start)/1000000,
					query.Name,
					result.Part,
					humanize.Bytes(uint64(len(result.Payload))),
					result.TotalRows,
				)
			}
			response.Columns = result.Columns
			responder.AddResponse(response)
		}
	}

	if uploader.Count > 0 {
		responder.Log("Uploaded %v files.", uploader.Count)
	}

	responder.Return()
}
