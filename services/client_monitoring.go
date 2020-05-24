// Velociraptor clients stream monitoring events to the server. This
// is controlled by the ClientEventTable below and can be updated by
// the GUI at any time.
//
// This service maintains access to the global event table.
package services

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	gEventTable = &ClientEventTable{}
)

// TODO: refresh event tables from data store periodically to pick up
// changes.

type ClientEventTable struct {
	// Version is protected by atomic mutations with a lock.
	version uint64 `json:"version"`

	ArtifactNames []string          `json:"artifacts"`
	Parameters    map[string]string `json:"parameters"`
	OpsPerSecond  float32           `json:"ops_per_second"`

	mu sync.Mutex

	job *crypto_proto.GrrMessage
}

func GetClientEventsVersion() uint64 {
	return atomic.LoadUint64(&gEventTable.version)
}

func UpdateClientEventTable(
	config_obj *config_proto.Config,
	args *flows_proto.ArtifactCollectorArgs) error {
	err := gEventTable.Update(config_obj, args)
	if err != nil {
		return err
	}

	// Notify all the client monitoring tables that we got
	// updated. This should cause all frontends to refresh.
	return NotifyListener(config_obj, constants.ClientMonitoringFlowURN)
}

func GetClientUpdateEventTableMessage() *crypto_proto.GrrMessage {
	return gEventTable.GetClientUpdateEventTableMessage()
}

func (self *ClientEventTable) GetClientUpdateEventTableMessage() *crypto_proto.GrrMessage {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Add a bit of randomness to the max wait to spread out
	// client's updates.
	result := proto.Clone(gEventTable.job).(*crypto_proto.GrrMessage)
	for _, event := range result.UpdateEventTable.Event {
		event.MaxWait += uint64(rand.Intn(20))
	}
	return result
}

// Start a new ClientEventTable with new rules.
func (self *ClientEventTable) Start(
	config_obj *config_proto.Config,
	version uint64,
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

	event_table := &actions_proto.VQLEventTable{
		Version: version,
	}

	rate := arg.OpsPerSecond
	if rate == 0 {
		rate = 1000
	}

	if arg.Artifacts != nil {
		for _, name := range arg.Artifacts {
			logger.Info("Collecting Client Monitoring Artifact: %s", name)

			vql_collector_args := &actions_proto.VQLCollectorArgs{
				MaxWait:      500,
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
	}

	self.job = &crypto_proto.GrrMessage{
		SessionId:        constants.MONITORING_WELL_KNOWN_FLOW,
		UpdateEventTable: event_table,
	}
	atomic.StoreUint64(&self.version, version)

	return nil
}

// Update the ClientEventTable with new rules then save them permanently.
func (self *ClientEventTable) Update(
	config_obj *config_proto.Config,
	arg *flows_proto.ArtifactCollectorArgs) error {

	// Increment the version to force clients to update their copy
	// of the event table.
	current_version := uint64(time.Now().Unix())

	// Store the new table in the data store.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	return db.SetSubject(config_obj, constants.ClientMonitoringFlowURN,
		&flows_proto.ClientEventTable{
			Version:   current_version,
			Artifacts: arg,
		})
}

func LoadFromFile(config_obj *config_proto.Config) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	event_table := flows_proto.ClientEventTable{
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			Artifacts:  []string{},
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
		artifacts := &flows_proto.ArtifactCollectorArgs{
			Artifacts: config_obj.Frontend.DefaultClientMonitoringArtifacts,
		}
		logger.Info("Creating default Client Monitoring Service")
		return gEventTable.Update(config_obj, artifacts)
	}

	return gEventTable.Start(
		config_obj, event_table.Version, event_table.Artifacts)
}

// Runs at frontend start to initialize the client monitoring table.
func StartClientMonitoringService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	notification, cancel := ListenForNotification(constants.ClientMonitoringFlowURN)
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return

			case <-notification:
				err := LoadFromFile(config_obj)
				if err != nil {
					logger.Error("StartClientMonitoringService: ", err)
					return
				}
			}

			cancel()
			notification, cancel = ListenForNotification(constants.ClientMonitoringFlowURN)
		}
	}()

	logger.Info("Starting Client Monitoring Service")
	return LoadFromFile(config_obj)
}
