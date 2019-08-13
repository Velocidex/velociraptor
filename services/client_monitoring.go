// Velociraptor clients stream monitoring events to the server. This
// is controlled by the ClientEventTable below and can be updated by
// the GUI at any time.
//
// This service maintains access to the global event table.
package services

import (
	"sync"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	gEventTable = &ClientEventTable{}
)

type ClientEventTable struct {
	// Version is protected by atomic mutations with a lock.
	version uint64 `json:"version"`

	ArtifactNames []string          `json:"artifacts"`
	Parameters    map[string]string `json:"parameters"`
	OpsPerSecond  float32           `json:"ops_per_second"`

	mu sync.Mutex

	// Not a pointer - getter gets a copy.
	flow_runner_args *flows_proto.FlowRunnerArgs
}

func GetClientEventsVersion() uint64 {
	return atomic.LoadUint64(&gEventTable.version)
}

// Returns an immutable copy of the flow runner args.
func GetClientEventsFlowRunnerArgs() *flows_proto.FlowRunnerArgs {
	return gEventTable.GetClientEventsFlowRunnerArgs()
}

func UpdateClientEventTable(
	config_obj *config_proto.Config,
	args *flows_proto.ArtifactCollectorArgs) error {
	return gEventTable.Update(config_obj, args)
}

func (self *ClientEventTable) GetClientEventsFlowRunnerArgs() *flows_proto.FlowRunnerArgs {
	self.mu.Lock()
	defer self.mu.Unlock()

	return proto.Clone(gEventTable.flow_runner_args).(*flows_proto.FlowRunnerArgs)
}

func (self *ClientEventTable) Update(
	config_obj *config_proto.Config,
	arg *flows_proto.ArtifactCollectorArgs) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	self.ArtifactNames = []string{}
	self.Parameters = make(map[string]string)
	self.OpsPerSecond = arg.OpsPerSecond

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	// Increment the version to force clients to update their copy
	// of the event table.
	current_version := atomic.LoadUint64(&self.version)
	current_version += 1
	atomic.StoreUint64(&self.version, current_version)

	event_table := &actions_proto.VQLEventTable{
		Version: current_version,
	}

	rate := arg.OpsPerSecond
	if rate == 0 {
		rate = 100
	}

	for _, name := range arg.Artifacts.Names {
		logger.Info("Collecting Client Monitoring Artifact: %s", name)

		vql_collector_args := &actions_proto.VQLCollectorArgs{
			MaxWait:      100,
			OpsPerSecond: rate,

			// Event queries never time out on their own.
			Timeout: 1000000000,
		}

		artifact, pres := repository.Get(name)
		if !pres {
			return errors.New("Unknown artifact " + name)
		}

		err := repository.Compile(artifact, vql_collector_args)
		if err != nil {
			return err
		}

		// Add any artifact dependencies.
		err = repository.PopulateArtifactsVQLCollectorArgs(vql_collector_args)
		if err != nil {
			return err
		}

		event_table.Event = append(event_table.Event, vql_collector_args)

		// Compress the VQL on the way out.
		err = artifacts.Obfuscate(config_obj, vql_collector_args)
		if err != nil {
			return err
		}
	}

	self.flow_runner_args = &flows_proto.FlowRunnerArgs{
		FlowName: "MonitoringFlow",
	}
	flow_args, err := ptypes.MarshalAny(event_table)
	if err != nil {
		return errors.WithStack(err)
	}
	self.flow_runner_args.Args = flow_args

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubject(
		config_obj, constants.ClientMonitoringFlowURN,
		&flows_proto.ClientEventTable{
			Version:   current_version,
			Artifacts: arg,
		})
	if err != nil {
		return err
	}

	return nil
}

// Runs at frontend start to initialize the client monitoring table.
func startClientMonitoringService(config_obj *config_proto.Config) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	event_table := flows_proto.ClientEventTable{
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			Artifacts:  &flows_proto.Artifacts{},
			Parameters: &flows_proto.ArtifactParameters{},
		},
	}
	err = db.GetSubject(
		config_obj,
		constants.ClientMonitoringFlowURN,
		&event_table)
	if err != nil {
		// No client monitoring rules found, install some
		// defaults.
		event_table.Artifacts.Artifacts.Names = []string{
			// Essential for client resource telemetry.
			"Generic.Client.Stats",

			// Very useful for process execution logging.
			"Windows.Events.ProcessCreation",
		}

		err = db.SetSubject(
			config_obj, constants.ClientMonitoringFlowURN,
			&event_table)
		if err != nil {
			return err
		}
	}

	logger := logging.GetLogger(
		config_obj, &logging.FrontendComponent)
	logger.Info("Starting Client Monitoring Service")
	return gEventTable.Update(config_obj, event_table.Artifacts)
}
