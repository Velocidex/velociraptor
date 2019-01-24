package api

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func streamQuery(
	config_obj *api_proto.Config,
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

	env := vfilter.NewDict().
		Set("config", config_obj).
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	for _, env_spec := range arg.Env {
		env.Set(env_spec.Key, env_spec.Value)
	}

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}
	repository.PopulateArtifactsVQLCollectorArgs(arg)

	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = logging.NewPlainLogger(config_obj,
		&logging.APICmponent)

	// All the queries will use the same scope. This allows one
	// query to define functions for the next query in order.
	for query_idx, query := range arg.Query {
		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			return err
		}
		result_chan := vfilter.GetResponseChannel(
			vql, stream.Context(), scope, int(arg.MaxRow), int(arg.MaxWait))
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
				Columns:   result.Columns,
			}
			if err := stream.Send(response); err != nil {
				return err
			}
		}
	}
	return nil
}
