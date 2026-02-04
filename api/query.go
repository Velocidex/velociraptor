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
package api

import (
	"context"
	"fmt"
	"io"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/dustin/go-humanize"
	errors "github.com/go-errors/errors"

	"github.com/sirupsen/logrus"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/executor/throttler"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

func streamQuery(
	ctx context.Context,
	config_obj *config_proto.Config,
	arg *actions_proto.VQLCollectorArgs,
	stream api_proto.API_QueryServer,
	peer_name string) (err error) {

	logger := logging.GetLogger(config_obj, &logging.APICmponent)
	logger.WithFields(logrus.Fields{
		"arg":  arg,
		"org":  arg.OrgId,
		"user": peer_name,
	}).Info("Query API call")

	if arg.MaxWait == 0 {
		arg.MaxWait = 10
	}

	if arg.Query == nil {
		return errors.New("Query should be specified.")
	}

	// If we panic we need to recover and report this to the
	// server.
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprintf("Panic %v", string(debug.Stack())))
		}
	}()

	response_channel := make(chan *actions_proto.VQLResponse)
	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	scope_logger := MakeLogger(sub_ctx, response_channel)

	// Add extra artifacts to the query from the global repository.
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: acl_managers.NewServerACLManager(config_obj, peer_name),
		Logger:     scope_logger,
		Repository: repository,
		Env:        ordereddict.NewDict(),
	}

	for _, env_spec := range arg.Env {
		builder.Env.Set(env_spec.Key, env_spec.Value)
	}

	// Now execute the query.
	scope := manager.BuildScope(builder)

	// Keep the connection open until the logs are sent.
	wg := sync.WaitGroup{}
	defer wg.Wait()

	// Implement timeout
	if arg.Timeout > 0 {
		start := time.Now()
		timed_ctx, timed_cancel := utils.WithTimeoutCause(sub_ctx,
			time.Second*time.Duration(arg.Timeout),
			errors.New("Query API timeout reached"))

		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			// Cancelling the parent will not return a log.
			case <-sub_ctx.Done():
				timed_cancel()

				// Log the timeout
			case <-timed_ctx.Done():
				scope.Log("collect: <red>Timeout Error:</> Collection timed out after %v",
					utils.GetTime().Now().Sub(start))
				// Cancel the main context.
				cancel()
				timed_cancel()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(response_channel)
		defer scope.Close()
		defer cancel()

		// Throttle the query if required. This must run in a
		// goroutine so it can emit logs otherwise we deadlock!
		t, closer := throttler.NewThrottler(sub_ctx, scope, config_obj,
			0, float64(arg.CpuLimit), 0)
		scope.SetThrottler(t)
		err = scope.AddDestructor(closer)
		if err != nil {
			closer()
			return
		}

		scope.Log("Starting query execution.")

		for _, query := range arg.Query {
			statements, err := vfilter.MultiParse(query.VQL)
			if err != nil {
				scope.Log("VQL Error: %v.", err)
				return
			}

			query_start := uint64(time.Now().UTC().UnixNano() / 1000)

			// All the queries will use the same scope. This allows one
			// query to define functions for the next query in order.
			for query_idx, vql := range statements {
				logger.Info("Query: Running %v\n",
					vfilter.FormatToString(scope, vql))

				result_chan := vfilter.GetResponseChannel(
					vql, sub_ctx, scope,
					vql_subsystem.MarshalJson(scope),
					int(arg.MaxRow), int(arg.MaxWait))

				for result := range result_chan {
					// Skip let queries since they never produce results.
					if vql.Let != "" {
						continue
					}

					response := &actions_proto.VQLResponse{
						Query:     query,
						QueryId:   uint64(query_idx),
						Part:      uint64(result.Part),
						Response:  string(result.Payload),
						Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
						Columns:   result.Columns,
					}

					scope.Log(
						"Time %v: %s: Sending response part %d %s (%d rows).",
						(response.Timestamp-query_start)/1000000,
						response.Query.Name,
						result.Part,
						humanize.Bytes(uint64(len(result.Payload))),
						result.TotalRows,
					)

					response_channel <- response
				}
			}
		}
	}()

	for response := range response_channel {
		err := stream.Send(response)
		if err != nil {
			return err
		}
	}

	return nil
}

type logWriter struct {
	output chan<- *actions_proto.VQLResponse
	ctx    context.Context
}

func (self *logWriter) Write(b []byte) (int, error) {
	// Sometimes the channel becomes closed for some reason and this
	// tends to panic.
	defer utils.CheckForPanic("logWriter.Write")

	select {
	case <-self.ctx.Done():
		return 0, io.EOF
	default:
	}

	select {
	case <-self.ctx.Done():
		return 0, io.EOF

	case self.output <- &actions_proto.VQLResponse{
		Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
		Log:       string(b),
	}:
	}
	return len(b), nil
}

func MakeLogger(ctx context.Context, output chan *actions_proto.VQLResponse) *log.Logger {
	result := &logWriter{output: output, ctx: ctx}
	return log.New(result, "", 0)
}
