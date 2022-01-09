/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package actions

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	humanize "github.com/dustin/go-humanize"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/uploads"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type LogWriter struct {
	config_obj *config_proto.Config
	responder  *responder.Responder
	ctx        context.Context
}

func (self *LogWriter) Write(b []byte) (int, error) {
	logging.GetLogger(self.config_obj, &logging.ClientComponent).Info("%v", string(b))
	self.responder.Log(self.ctx, "%s", string(b))
	return len(b), nil
}

type VQLClientAction struct{}

func (self VQLClientAction) StartQuery(
	config_obj *config_proto.Config,
	ctx context.Context,
	responder *responder.Responder,
	arg *actions_proto.VQLCollectorArgs) {

	// Set reasonable defaults.
	max_wait := arg.MaxWait
	if max_wait == 0 {
		max_wait = config_obj.Client.DefaultMaxWait

		if max_wait == 0 {
			max_wait = 100
		}
	}

	max_row := arg.MaxRow
	if max_row == 0 {
		max_row = 10000
	}

	rate := arg.OpsPerSecond
	if rate == 0 {
		rate = 1000000
	}

	timeout := arg.Timeout
	if timeout == 0 {
		timeout = 600
	}

	heartbeat := arg.Heartbeat
	if heartbeat == 0 {
		heartbeat = 30
	}

	// Cancel the query after this deadline
	deadline := time.After(time.Second * time.Duration(timeout))
	started := time.Now().Unix()
	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if len(arg.Query) == 0 {
		responder.RaiseError(ctx, "Query should be specified.")
		return
	}

	// Clients do not have a copy of artifacts so they need to be
	// sent all artifacts from the server.
	manager, err := services.GetRepositoryManager()
	if err != nil {
		responder.RaiseError(ctx, fmt.Sprintf("%v", err))
		return
	}

	repository := manager.NewRepository()
	for _, artifact := range arg.Artifacts {
		artifact.BuiltIn = false
		_, err := repository.LoadProto(artifact, true /* validate */)
		if err != nil {
			responder.RaiseError(ctx, fmt.Sprintf(
				"Failed to compile artifact %v.", artifact.Name))
			return
		}
	}

	uploader := &uploads.VelociraptorUploader{
		Responder: responder,
	}

	builder := services.ScopeBuilder{
		// Only provide the client config since we are running in
		// client context.
		ClientConfig: config_obj.Client,
		// Disable ACLs on the client.
		ACLManager: vql_subsystem.NullACLManager{},
		Env:        ordereddict.NewDict(),
		Uploader:   uploader,
		Repository: repository,
		Logger:     log.New(&LogWriter{config_obj, responder, ctx}, "vql: ", 0),
	}

	for _, env_spec := range arg.Env {
		builder.Env.Set(env_spec.Key, env_spec.Value)
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	if runtime.GOARCH == "386" &&
		os.Getenv("PROCESSOR_ARCHITEW6432") == "AMD64" {
		scope.Log("You are running a 32 bit built binary on Windows x64. " +
			"This configuration is not supported and may result in " +
			"incorrect or missed results or even crashes.")
	}

	scope.Log("Starting query execution.")

	vfilter.InstallThrottler(scope, vfilter.NewTimeThrottler(float64(rate)))

	start := time.Now()

	// If we panic we need to recover and report this to the
	// server.
	defer func() {
		r := recover()
		if r != nil {
			msg := string(debug.Stack())
			scope.Log(msg)
			responder.RaiseError(ctx, msg)
		}

		scope.Log("Collection is done after %v", time.Since(start))
	}()

	ok, err := CheckPreconditions(ctx, scope, arg)
	if err != nil {
		scope.Log("While evaluating preconditions: %v", err)
		responder.RaiseError(ctx, fmt.Sprintf("While evaluating preconditions: %v", err))
		return
	}

	if !ok {
		scope.Log("Skipping query due to preconditions")
		responder.Return(ctx)
		return
	}

	// All the queries will use the same scope. This allows one
	// query to define functions for the next query in order.
	for query_idx, query := range arg.Query {
		query_log := QueryLog.AddQuery(query.VQL)

		query_start := uint64(time.Now().UTC().UnixNano() / 1000)
		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			responder.RaiseError(ctx, err.Error())
			query_log.Close()
			return
		}

		result_chan := vfilter.GetResponseChannel(
			vql, sub_ctx, scope,
			vql_subsystem.MarshalJsonl(scope),
			int(max_row),
			int(max_wait))
	run_query:
		for {
			select {
			case <-deadline:
				msg := fmt.Sprintf("Query timed out after %v seconds",
					time.Now().Unix()-started)
				scope.Log(msg)

				// Queries that time out are an error on the server.
				responder.RaiseError(ctx, msg)

				// Cancel the sub ctx but do not exit
				// - we need to wait for the sub query
				// to finish after cancelling so we
				// can at least return any data it
				// has.
				cancel()
				scope.Close()

				// Try again after a while to prevent spinning here.
				deadline = time.After(time.Second * time.Duration(timeout))

			case <-time.After(time.Second * time.Duration(heartbeat)):
				responder.Log(ctx, "Time %v: %s: Waiting for rows.",
					(uint64(time.Now().UTC().UnixNano()/1000)-
						query_start)/1000000, query.Name)

			case result, ok := <-result_chan:
				if !ok {
					query_log.Close()
					break run_query
				}
				// Skip let queries since they never produce results.
				if strings.HasPrefix(strings.ToLower(query.VQL), "let") {
					continue
				}
				response := &actions_proto.VQLResponse{
					Query:         query,
					QueryId:       uint64(query_idx),
					Part:          uint64(result.Part),
					JSONLResponse: string(result.Payload),
					TotalRows:     uint64(result.TotalRows),
					Timestamp:     uint64(time.Now().UTC().UnixNano() / 1000),
				}

				// Don't log empty VQL statements.
				if query.Name != "" {
					responder.Log(ctx,
						"Time %v: %s: Sending response part %d %s (%d rows).",
						(response.Timestamp-query_start)/1000000,
						query.Name,
						result.Part,
						humanize.Bytes(uint64(len(result.Payload))),
						result.TotalRows,
					)
				}
				response.Columns = result.Columns
				responder.AddResponse(ctx, &crypto_proto.VeloMessage{
					VQLResponse: response})
			}
		}
	}

	if uploader.Count > 0 {
		responder.Log(ctx, "Uploaded %v files.", uploader.Count)
	}

	responder.Return(ctx)
}

func CheckPreconditions(
	ctx context.Context,
	scope vfilter.Scope,
	arg *actions_proto.VQLCollectorArgs) (bool, error) {

	// No precondition means the query is allowed.
	if arg.Precondition == "" {
		return true, nil
	}

	vqls, err := vfilter.MultiParse(arg.Precondition)
	if err != nil {
		return false, err
	}

	for _, vql := range vqls {
		for _ = range vql.Eval(ctx, scope) {
			return true, nil
		}
	}
	return false, nil
}
