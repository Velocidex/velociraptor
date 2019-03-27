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
package services

import (
	"context"
	"errors"
	"path"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ParticipationRecord struct {
	HuntId      string `vfilter:"required,field=HuntId"`
	ClientId    string `vfilter:"required,field=ClientId"`
	Fqdn        string `vfilter:"optional,field=Fqdn"`
	FlowId      string `vfilter:"optional,field=FlowId"`
	Participate bool   `vfilter:"required,field=Participate"`
}

type HuntManager struct {
	// We keep a cache of hunt writers to write the output of each
	// hunt. Note that each writer is responsible for its own
	// flushing etc.
	writers    map[string]*csv.CSVWriter
	wg         sync.WaitGroup
	done       chan bool
	config_obj *api_proto.Config
}

func (self *HuntManager) Start() error {
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Starting hunt manager.")

	env := vfilter.NewDict().
		Set("config", self.config_obj.Client).
		Set("server_config", self.config_obj)

	repository, err := artifacts.GetGlobalRepository(self.config_obj)
	if err != nil {
		return err
	}
	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = logging.NewPlainLogger(self.config_obj,
		&logging.FrontendComponent)

	vql, err := vfilter.Parse("select HuntId, ClientId, Fqdn, Participate FROM " +
		"watch_monitoring(artifact='System.Hunt.Participation')")
	if err != nil {
		return err
	}
	self.wg.Add(1)

	go func() {
		defer self.wg.Done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		row_chan := vql.Eval(ctx, scope)
		for {
			select {
			case <-self.done:
				return

			case row, ok := <-row_chan:
				if !ok {
					return
				}
				self.ProcessRow(scope, row)
			}
		}
	}()

	return nil
}

// Close will block until all our cleanup is done.
func (self *HuntManager) Close() {
	for _, v := range self.writers {
		v.Close()
	}
	close(self.done)
	self.wg.Wait()
}

func (self *HuntManager) ProcessRow(
	scope *vfilter.Scope,
	row vfilter.Row) {

	dict_row := vql_subsystem.RowToDict(scope, row)
	participation_row := &ParticipationRecord{}
	err := vfilter.ExtractArgs(scope, dict_row, participation_row)
	if err != nil {
		scope.Log("ExtractArgs %v", err)
		return
	}

	// The client will not participate in this hunt - nothing to do.
	if !participation_row.Participate {
		return
	}

	// Fetch the CSV writer for this hunt or create a new one and
	// cache it.
	writer, pres := self.writers[participation_row.HuntId]
	if !pres {
		file_store_factory := file_store.GetFileStore(self.config_obj)
		fd, err := file_store_factory.WriteFile(
			participation_row.HuntId + ".csv")
		if err != nil {
			return
		}

		writer, err = csv.GetCSVWriter(scope, fd)
		if err != nil {
			return
		}

		self.writers[participation_row.HuntId] = writer
	}

	request := &flows_proto.FlowRunnerArgs{
		ClientId: participation_row.ClientId,
		Creator:  participation_row.HuntId,
	}

	// Get hunt information about this hunt.
	now := uint64(time.Now().UnixNano() / 1000)
	err = GetHuntDispatcher().ModifyHunt(
		path.Base(participation_row.HuntId),
		func(hunt_obj *api_proto.Hunt) error {
			// Ignore stopped hunts.
			if hunt_obj.Stats.Stopped ||
				hunt_obj.State != api_proto.Hunt_RUNNING {
				return errors.New("hunt is stopped")
			}

			// Ignore hunts with label conditions which
			// exclude this client.
			if !huntHasLabel(
				self.config_obj,
				hunt_obj,
				participation_row.ClientId) {
				return errors.New("hunt label does not match")
			}

			// Hunt limit exceeded or it expired - we stop it.
			if (hunt_obj.ClientLimit > 0 &&
				hunt_obj.Stats.TotalClientsScheduled >= hunt_obj.ClientLimit) ||
				now > hunt_obj.Expires {

				// Stop the hunt.
				hunt_obj.Stats.Stopped = true
				return errors.New("hunt is expired")
			}

			// Use hunt information to launch the flow
			// against this client.
			proto.Merge(request, hunt_obj.StartRequest)
			hunt_obj.Stats.TotalClientsScheduled += 1

			return nil
		})

	if err != nil {
		scope.Log("hunt manager: launching %v:  %v", participation_row, err)
		return
	}

	// Issue the flow on the client.
	channel := grpc_client.GetChannel(self.config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	response, err := client.LaunchFlow(context.Background(), request)
	if err != nil {
		scope.Log("hunt manager: %s", err.Error())
		return
	}

	dict_row.Set("FlowId", response.FlowId)
	dict_row.Set("Timestamp", time.Now().Unix())
	writer.Write(dict_row)
}

func startHuntManager(config_obj *api_proto.Config) (
	*HuntManager, error) {
	result := &HuntManager{
		config_obj: config_obj,
		writers:    make(map[string]*csv.CSVWriter),
		done:       make(chan bool),
		wg:         sync.WaitGroup{},
	}
	err := result.Start()
	return result, err
}

func huntHasLabel(config_obj *api_proto.Config,
	hunt_obj *api_proto.Hunt,
	client_id string) bool {

	label_condition := hunt_obj.Condition.GetLabels()
	if label_condition != nil && len(label_condition.Label) > 0 {
		channel := grpc_client.GetChannel(config_obj)
		defer channel.Close()

		client := api_proto.NewAPIClient(channel)
		request := &api_proto.LabelClientsRequest{
			ClientIds: []string{client_id},
			Labels:    label_condition.Label,
			Operation: "check",
		}
		_, err := client.LabelClients(
			context.Background(), request)

		if err != nil {
			return false
		}
	}

	return true
}
