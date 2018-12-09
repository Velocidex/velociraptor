package server

import (
	"context"
	"encoding/json"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/urns"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type CollectedArtifactsPluginArgs struct {
	ClientId []string `vfilter:"required,field=client_id"`
	Artifact string   `vfilter:"required,field=artifact"`
}

type CollectedArtifactsPlugin struct{}

func (self CollectedArtifactsPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

		arg := &CollectedArtifactsPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("collected_artifacts: %v", err)
			return
		}

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*api_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}

		for _, client_id := range arg.ClientId {
			flow_urns, err := db.ListChildren(
				config_obj, urns.BuildURN(
					"clients", client_id, "flows"),
				0, 10000)
			if err != nil {
				return
			}

			for _, urn := range flow_urns {
				flow_obj, err := flows.GetAFF4FlowObject(config_obj, urn)
				if err != nil {
					continue
				}

				if flow_obj.RunnerArgs.FlowName != "ArtifactCollector" {
					continue
				}

				result_urns, err := db.ListChildren(
					config_obj, urns.BuildURN(urn, "results"), 0, 10000)
				if err != nil {
					return
				}

				for _, result_urn := range result_urns {
					message := &crypto_proto.GrrMessage{}
					err := db.GetSubject(config_obj, result_urn, message)
					if err != nil || message.ArgsRdfName != "VQLResponse" {
						continue
					}

					payload := responder.ExtractGrrMessagePayload(
						message).(*actions_proto.VQLResponse)
					if payload.Query.Name != "Artifact "+arg.Artifact {
						continue
					}

					data := []map[string]interface{}{}
					err = json.Unmarshal([]byte(payload.Response), &data)
					if err != nil {
						continue
					}

					for _, row := range data {
						new_row := vfilter.NewDict().
							Set("ClientId", client_id).
							Set("CollectedTime", payload.Timestamp)

						for _, k := range payload.Columns {
							item, pres := row[k]
							if !pres {
								item = vfilter.Null{}
							}
							new_row.Set(k, item)
						}

						output_chan <- new_row
					}
				}
			}
		}

	}()

	return output_chan
}

func (self CollectedArtifactsPlugin) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "collected_artifact",
		Doc:     "Retrieve artifacts collected from clients.",
		ArgType: type_map.AddType(scope, &CollectedArtifactsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&CollectedArtifactsPlugin{})
}
