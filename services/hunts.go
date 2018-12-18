package services

import (
	"context"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ParticipationRecord struct {
	HuntId      string `vfilter:"required,field=HuntId"`
	ClientId    string `vfilter:"required,field=ClientId"`
	FlowId      string `vfilter:"optional,field=FlowId"`
	Participate bool   `vfilter:"required,field=Participate"`
}

type HuntManager struct {
	// We keep a cache of hunt writers to write the output of each
	// hunt. Note that each writer is responsible for its own
	// flushing etc.
	writers    map[string]*csv.CSVWriter
	hunts      map[string]*api_proto.Hunt
	wg         sync.WaitGroup
	done       chan bool
	scope      *vfilter.Scope
	config_obj *api_proto.Config
}

func (self *HuntManager) Start() error {
	logger := logging.NewLogger(self.config_obj)
	logger.Info("Starting hunt manager.")

	env := vfilter.NewDict().
		Set("config", self.config_obj.Client).
		Set("server_config", self.config_obj)

	repository, err := artifacts.GetGlobalRepository(self.config_obj)
	if err != nil {
		return err
	}
	scope := artifacts.MakeScope(repository).AppendVars(env)
	scope.Logger = logging.NewPlainLogger(self.config_obj)

	vql, err := vfilter.Parse("select HuntId, ClientId, Participate FROM " +
		"watch_monitoring(artifact='System.Hunt.Participation')")
	if err != nil {
		return err
	}

	go func() {
		self.wg.Add(1)
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

	// Get hunt information about this hunt.
	hunt_obj, pres := self.hunts[participation_row.HuntId]
	if !pres {
		db, err := datastore.GetDB(self.config_obj)
		if err != nil {
			return
		}

		hunt_obj = &api_proto.Hunt{}
		err = db.GetSubject(
			self.config_obj, participation_row.HuntId, hunt_obj)
		if err != nil {
			return
		}

		self.hunts[participation_row.HuntId] = hunt_obj
	}

	// Use hunt information to launch the flow against this
	// client.
	request := *hunt_obj.StartRequest
	request.ClientId = participation_row.ClientId

	// Issue the flow on the client.
	channel := grpc_client.GetChannel(self.config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	response, err := client.LaunchFlow(context.Background(), &request)
	if err != nil {
		scope.Log("hunt manager: %s", err.Error())
		return
	}

	dict_row.Set("FlowId", response.FlowId)
	writer.Write(dict_row)
}

func StartHuntManager(config_obj *api_proto.Config) (
	*HuntManager, error) {
	result := &HuntManager{
		config_obj: config_obj,
		writers:    make(map[string]*csv.CSVWriter),
		done:       make(chan bool),
		hunts:      make(map[string]*api_proto.Hunt),
		wg:         sync.WaitGroup{},
	}
	err := result.Start()
	return result, err
}
