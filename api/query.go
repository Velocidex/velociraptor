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
package api

import (
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/dustin/go-humanize"
	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func streamQuery(
	config_obj *config_proto.Config,
	arg *actions_proto.VQLCollectorArgs,
	stream api_proto.API_QueryServer,
	peer_name string) (err error) {

	logger := logging.GetLogger(config_obj, &logging.APICmponent)
	logger.WithFields(logrus.Fields{
		"arg":  arg,
		"user": peer_name,
	}).Info("Query API call")

	if arg.MaxWait == 0 {
		arg.MaxWait = 10
	}

	rate := arg.OpsPerSecond
	if rate == 0 {
		rate = 1000000
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
	scope_logger := MakeLogger(response_channel)

	// Add extra artifacts to the query from the global repository.
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NewServerACLManager(config_obj, peer_name),
		Logger:     scope_logger,
		Repository: repository,
		Env:        ordereddict.NewDict(),
	}

	for _, env_spec := range arg.Env {
		builder.Env.Set(env_spec.Key, env_spec.Value)
	}

	// Now execute the query.
	scope := manager.BuildScope(builder)
	defer scope.Close()

	// Throttle the query if required.
	vfilter.InstallThrottler(scope, vfilter.NewTimeThrottler(float64(rate)))

	go func() {
		defer close(response_channel)

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
				fmt.Printf("Running %v\n", vql.ToString(scope))

				result_chan := vfilter.GetResponseChannel(
					vql, stream.Context(), scope,
					vql_subsystem.MarshalJson(scope),
					int(arg.MaxRow), int(arg.MaxWait))

				for result := range result_chan {
					fmt.Printf("Got %v\n", string(result.Payload))
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
}

func (self *logWriter) Write(b []byte) (int, error) {
	self.output <- &actions_proto.VQLResponse{
		Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
		Log:       string(b),
	}
	return len(b), nil
}

func MakeLogger(output chan *actions_proto.VQLResponse) *log.Logger {
	result := &logWriter{output: output}
	return log.New(result, "vql: ", 0)
}
