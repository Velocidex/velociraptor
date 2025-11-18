/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"bytes"
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
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/executor/throttler"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type LogWriter struct {
	config_obj *config_proto.Config
	responder  responder.Responder
	ctx        context.Context
}

func NewLogWriter(
	ctx context.Context,
	config_obj *config_proto.Config,
	responder responder.Responder) *LogWriter {
	return &LogWriter{
		ctx:        ctx,
		config_obj: config_obj,
		responder:  responder,
	}
}

func (self *LogWriter) Write(b []byte) (int, error) {
	level, msg := logging.SplitIntoLevelAndLog(b)

	self.responder.Log(self.ctx, level, msg)
	logging.GetLogger(self.config_obj, &logging.ClientComponent).
		LogWithLevel(level, "%v", msg)
	return len(b), nil
}

type VQLClientAction struct{}

func (self VQLClientAction) StartQuery(
	config_obj *config_proto.Config,
	ctx context.Context,
	responder responder.Responder,
	arg *actions_proto.VQLCollectorArgs) {

	defer responder.Return(ctx)

	// Just ignore requests that are too old.
	if arg.Expiry > 0 && arg.Expiry < uint64(utils.Now().Unix()) {
		responder.RaiseError(ctx, "Query expired.")
		return
	}

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

	max_row_buffer_size := arg.MaxRowBufferSize
	if max_row_buffer_size == 0 {
		max_row_buffer_size = 5 * 1024 * 1024
	}

	rate := arg.OpsPerSecond
	cpu_limit := arg.CpuLimit
	iops_limit := arg.IopsLimit

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
	started := utils.Now().Unix()
	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if len(arg.Query) == 0 {
		responder.RaiseError(ctx, "Query should be specified.")
		return
	}

	name := strings.Split(utils.GetQueryName(arg.Query), "/")[0]

	// Clients do not have a copy of artifacts so they need to be
	// sent all artifacts from the server.
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		responder.RaiseError(ctx, fmt.Sprintf("%v", err))
		return
	}

	repository := manager.NewRepository()
	for _, artifact := range arg.Artifacts {
		artifact.BuiltIn = false
		_, err := repository.LoadProto(artifact,
			services.ArtifactOptions{
				ValidateArtifact: true,
			})
		if err != nil {
			responder.RaiseError(ctx, fmt.Sprintf(
				"Failed to compile artifact %v.", artifact.Name))
			return
		}
	}

	logger := log.New(NewLogWriter(ctx, config_obj, responder), "", 0)
	uploader := uploads.NewVelociraptorUploader(sub_ctx, logger,
		time.Duration(timeout)*time.Second, responder)
	defer uploader.Close()

	builder := services.ScopeBuilder{
		Config: &config_proto.Config{
			Client:     config_obj.Client,
			Remappings: config_obj.Remappings,
		},
		Ctx: ctx,

		// Only provide the client config since we are running in
		// client context.
		ClientConfig: config_obj.Client,
		// Disable ACLs on the client.
		ACLManager: acl_managers.NullACLManager{},
		Env: ordereddict.NewDict().
			// Make the session id available in the query.
			Set("_SessionId", responder.FlowContext().SessionId()).
			Set(constants.SCOPE_RESPONDER, responder),
		Uploader:   uploader,
		Repository: repository,
		Logger:     logger,
	}

	for _, env_spec := range arg.Env {
		builder.Env.Set(env_spec.Key, env_spec.Value)
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	// The uploader needs to be flushed before the scope is destroyed
	// because transactions may still be active.
	defer uploader.Close()

	// Allow VQL to gain access to the flow responder for low level
	// functionality.
	scope.SetContext(constants.SCOPE_RESPONDER_CONTEXT, responder)

	// Add some additional context for debugging
	scope.SetContext(constants.SCOPE_QUERY_NAME, name)

	if runtime.GOARCH == "386" &&
		os.Getenv("PROCESSOR_ARCHITEW6432") == "AMD64" {
		scope.Log("You are running a 32 bit built binary on Windows x64. " +
			"This configuration is not supported and may result in " +
			"incorrect or missed results or even crashes.")
	}

	scope.Log("INFO:Starting query execution for %v.", name)

	// Make a throttler
	throttler, closer := throttler.NewThrottler(ctx, scope, config_obj,
		float64(rate), float64(cpu_limit), float64(iops_limit))
	defer closer()

	if arg.ProgressTimeout > 0 {
		duration := time.Duration(arg.ProgressTimeout) * time.Second
		throttler = NewProgressThrottler(
			sub_ctx, scope, cancel, throttler, duration)
		scope.Log("query: Installing a progress alarm for %v", duration)
	}
	scope.SetThrottler(throttler)

	start := utils.Now()

	// If we panic we need to recover and report this to the
	// server.
	defer func() {
		r := recover()
		if r != nil {
			msg := string(debug.Stack())
			scope.Log(msg)
			responder.RaiseError(ctx, msg)
		}

		scope.Log("INFO:Collection %v is done after %v", name, time.Since(start))
	}()

	ok, err := CheckPreconditions(ctx, scope, arg)
	if err != nil {
		scope.Log("%v: While evaluating preconditions: %v", name, err)
		responder.RaiseError(ctx,
			fmt.Sprintf("While evaluating preconditions: %v", err))
		return
	}

	if !ok {
		scope.Log("INFO:%v: Skipping query due to preconditions", name)
		responder.Return(ctx)
		return
	}

	row_tracker := NewQueryTracker()

	// All the queries will use the same scope. This allows one
	// query to define functions for the next query in order.
	for query_idx, query := range arg.Query {
		query_log := QueryLog.AddQuery(query.VQL)

		query_start := uint64(utils.Now().UTC().UnixNano() / 1000)
		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			responder.RaiseError(ctx, err.Error())
			query_log.Close()
			return
		}

		result_chan := EncodeIntoResponsePackets(
			vql, sub_ctx, scope,
			int(max_row), int(max_wait), int(max_row_buffer_size))
	run_query:
		for {
			select {
			case <-deadline:
				msg := fmt.Sprintf("Query timed out after %v seconds",
					utils.Now().Unix()-started)
				scope.Log(msg)

				// Queries that time out are an error on the server.
				responder.RaiseError(ctx, msg)

				// Cancel the sub ctx but do not exit
				// - we need to wait for the sub query
				// to finish after cancelling so we
				// can at least return any data it
				// has.
				cancel()
				uploader.Abort()

				scope.Close()

				// Try again after a while to prevent spinning here.
				deadline = time.After(time.Second * time.Duration(timeout))

			case <-time.After(time.Second * time.Duration(heartbeat)):
				responder.Log(ctx, logging.DEFAULT,
					fmt.Sprintf("%v: Time %v: %s: Waiting for rows.", name,
						(uint64(utils.Now().UTC().UnixNano()/1000)-
							query_start)/1000000, query.Name))

			case result, ok := <-result_chan:
				if !ok {
					query_log.Close()
					break run_query
				}

				response := &actions_proto.VQLResponse{
					Query:         query,
					QueryId:       uint64(query_idx),
					Part:          uint64(result.Part),
					JSONLResponse: string(result.Payload),
					TotalRows:     uint64(result.TotalRows),
					QueryStartRow: row_tracker.GetStartRow(query),
					Timestamp:     uint64(utils.Now().UTC().UnixNano() / 1000),
				}

				// Do not send empty responses
				if result.TotalRows == 0 {
					continue
				}

				row_tracker.AddRows(query, uint64(result.TotalRows))

				// Don't log empty VQL statements.
				if query.Name != "" {
					responder.Log(ctx,
						logging.DEFAULT,
						fmt.Sprintf(
							"%v: Time %v: %s: Sending response part %d %s (%d rows).",
							name,
							(response.Timestamp-query_start)/1000000,
							query.Name,
							result.Part,
							humanize.Bytes(uint64(len(result.Payload))),
							result.TotalRows,
						))
				}
				response.Columns = result.Columns
				responder.AddResponse(&crypto_proto.VeloMessage{
					VQLResponse: response})
			}
		}
	}

	if uploader.GetCount() > 0 {
		if uploader.GetTransactionCount() > 0 {
			responder.Log(ctx, logging.DEFAULT,
				fmt.Sprintf("%v: Uploaded %v files with %v outstanding upload transactions.",
					name, uploader.GetCount(),
					uploader.GetTransactionCount()))
		} else {
			responder.Log(ctx, logging.DEFAULT,
				fmt.Sprintf("%v: Uploaded %v files.",
					name, uploader.GetCount()))
		}
	}
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

func EncodeIntoResponsePackets(
	vql *vfilter.VQL,
	ctx context.Context,
	scope types.Scope,
	maxrows int,
	// Max time to wait before returning some results.
	max_wait int,
	// How large do we allow the payload to get
	max_row_buffer_size int) <-chan *vfilter.VFilterJsonResult {
	result_chan := make(chan *vfilter.VFilterJsonResult)

	encoder := vql_subsystem.MarshalJsonl(scope)

	go func() {
		defer close(result_chan)

		part := 0
		row_chan := vql.Eval(ctx, scope)
		buffer := bytes.Buffer{}
		var columns []string
		var total_rows int

		ship_payload := func() {
			result := &vfilter.VFilterJsonResult{
				Part:      part,
				TotalRows: total_rows,
				Payload:   buffer.Bytes(),
			}

			total_rows = 0
			// Use a NEW buffer here to avoid trashing the byte slice
			// above. See
			// https://github.com/Velocidex/velociraptor/issues/1793
			buffer = bytes.Buffer{}

			result.Columns = columns
			result_chan <- result
			part += 1
		}

		// Send the last payload outstanding.
		defer ship_payload()

		// First deadline is max_wait in the future
		deadline := time.After(time.Duration(max_wait) * time.Second)

		for {
			select {
			case <-ctx.Done():
				return

			// If the query takes too long, send what we
			// have.
			case <-deadline:
				if total_rows > 0 {
					ship_payload()
				}

				// Update the deadline to re-fire next.
				deadline = time.After(time.Duration(max_wait) * time.Second)

			case row, ok := <-row_chan:
				if !ok {
					return
				}

				// Materialize all elements if needed.
				value := vfilter.RowToDict(ctx, scope, row)

				// Set the columns according to the first row.
				if len(columns) == 0 {
					columns = value.Keys()
				}

				// Encode the row into bytes ASAP so we can reclaim
				// memory.
				s, err := encoder([]types.Row{value})
				if err != nil {
					scope.Log("Unable to serialize: %v", err)
					return
				}
				// Accumulate the jsonl into the buffer
				total_rows++
				buffer.Write(s)

				// Send the payload if it is too full.
				if total_rows >= maxrows ||
					buffer.Len() > max_row_buffer_size {
					ship_payload()
					deadline = time.After(
						time.Duration(max_wait) * time.Second)
				}
			}
		}
	}()

	return result_chan
}
