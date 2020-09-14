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
}

func (self *LogWriter) Write(b []byte) (int, error) {
	logging.GetLogger(self.config_obj, &logging.ClientComponent).Info("%v", string(b))
	self.responder.Log("%s", string(b))
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

	// Cancel the query after this deadline
	deadline := time.After(time.Second * time.Duration(timeout))
	started := time.Now().Unix()
	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if arg.Query == nil {
		responder.RaiseError("Query should be specified.")
		return
	}

	// Clients do not have a copy of artifacts so they need to be
	// sent all artifacts from the server.
	manager, err := services.GetRepositoryManager()
	if err != nil {
		responder.RaiseError(fmt.Sprintf("%v", err))
		return
	}

	repository := manager.NewRepository()
	for _, artifact := range arg.Artifacts {
		_, err := repository.LoadProto(artifact, false /* validate */)
		if err != nil {
			responder.RaiseError(fmt.Sprintf(
				"Failed to compile artifact %v.", artifact.Name))
			return
		}
	}

	uploader := &uploads.VelociraptorUploader{
		Responder: responder,
	}

	builder := services.ScopeBuilder{
		Config: config_obj,
		// Disable ACLs on the client.
		ACLManager: vql_subsystem.NullACLManager{},
		Env:        ordereddict.NewDict(),
		Uploader:   uploader,
		Repository: repository,
		Logger:     log.New(&LogWriter{config_obj, responder}, "vql: ", 0),
	}

	for _, env_spec := range arg.Env {
		builder.Env.Set(env_spec.Key, env_spec.Value)
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	scope.Log("Starting query execution.")

	vfilter.InstallThrottler(scope, vfilter.NewTimeThrottler(float64(rate)))

	// If we panic we need to recover and report this to the
	// server.
	defer func() {
		r := recover()
		if r != nil {
			msg := string(debug.Stack())
			scope.Log(msg)
			responder.RaiseError(msg)
		}
	}()

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
				responder.RaiseError(msg)

				// Cancel the sub ctx but do not exit
				// - we need to wait for the sub query
				// to finish after cancelling so we
				// can at least return any data it
				// has.
				cancel()
				scope.Close()

				// Try again after a while to prevent spinning here.
				deadline = time.After(time.Second * time.Duration(timeout))

			case result, ok := <-result_chan:
				if !ok {
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
				responder.AddResponse(&crypto_proto.GrrMessage{
					VQLResponse: response})
			}
		}
	}

	if uploader.Count > 0 {
		responder.Log("Uploaded %v files.", uploader.Count)
	}

	responder.Return()
}
