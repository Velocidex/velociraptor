// This service is responsible for selecting the frontend to use. It
// should be called very early in the frontend start process.

// When we start up, we inspect the file store to find a frontend that
// is not currently running, and then we take over that frontend.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	currentReplicationConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "minion_replication_grpc_connections",
		Help: "Current number of connections to the master.",
	})
)

func PushMetrics(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config, node_name string) error {

	wg.Add(1)
	go func() {
		defer wg.Done()

		metrics := &FrontendMetrics{
			NodeName: node_name,
		}
		rows := make([]*ordereddict.Dict, 1)

		for {
			// Wait for 10 seconds between updates
			select {
			case <-ctx.Done():
				return

			case <-time.After(utils.Jitter(10 * time.Second)):
			}

			// Journal may not be ready yet so it is not
			// an error if its not there, just try again
			// later.
			journal, err := services.GetJournal(config_obj)
			if err != nil {
				continue
			}

			if calculateMetrics(metrics) == nil {
				rows[0] = ordereddict.NewDict().
					Set("Node", node_name).
					Set("Metrics", metrics.ToDict())
				_ = journal.PushRowsToArtifact(ctx, config_obj,
					rows, "Server.Internal.FrontendMetrics",
					"server", "")
			}
		}

	}()

	return nil
}

func calculateMetrics(metrics *FrontendMetrics) error {
	now := time.Now()

	// Time difference in nanosec
	delta_t := now.UnixNano() - metrics.Timestamp.UnixNano()
	gathering, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		metrics.Timestamp = now
		return err
	}

	for _, metric := range gathering {
		if len(metric.Metric) == 0 {
			continue
		}

		switch *metric.Name {
		case "process_cpu_seconds_total":
			if metric.Metric[0].Counter != nil {
				total_time := (int64)(*metric.Metric[0].Counter.Value * 1e9)

				delta_cpu := (total_time - metrics.ProcessCpuNanoSecondsTotal)

				if delta_t > 0 && metrics.Timestamp.UnixNano() > 0 {
					metrics.CpuLoadPercent =
						delta_cpu * 100 / delta_t
				}
				metrics.ProcessCpuNanoSecondsTotal = total_time
			}

		case "process_resident_memory_bytes":
			if metric.Metric[0].Gauge != nil {
				metrics.ProcessResidentMemoryBytes = (int64)(
					*metric.Metric[0].Gauge.Value)
			}
		}
	}

	metrics.Timestamp = now

	// Fill in org connections.
	return calculateOrgConnectionClients(metrics)
}

func calculateOrgConnectionClients(metric *FrontendMetrics) error {
	metric.ClientCommsCurrentConnections = make(map[string]uint64)

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}
	for _, org := range org_manager.ListOrgs() {
		org_config_obj, err := org_manager.GetOrgConfig(org.Id)
		if err != nil {
			continue
		}

		// Get the notifier for each org.
		notifier, err := services.GetNotifier(org_config_obj)
		if err != nil {
			continue
		}

		metric.ClientCommsCurrentConnections[org.Id] = notifier.CountConnectedClients()
	}

	return nil
}

type FrontendMetrics struct {
	Timestamp                     time.Time
	ProcessCpuNanoSecondsTotal    int64
	CpuLoadPercent                int64
	ClientCommsCurrentConnections map[string]uint64
	ProcessResidentMemoryBytes    int64
	NodeName                      string
}

func (self FrontendMetrics) ToDict() *ordereddict.Dict {
	return ordereddict.NewDict().
		Set("Timestamp", self.Timestamp).
		Set("ProcessCpuNanoSecondsTotal", self.ProcessCpuNanoSecondsTotal).
		Set("CpuLoadPercent", self.CpuLoadPercent).
		Set("ClientCommsCurrentConnections", self.ClientCommsCurrentConnections).
		Set("ProcessResidentMemoryBytes", self.ProcessResidentMemoryBytes).
		Set("NodeName", self.NodeName)
}

// The master frontend is responsible for aggregating minion stats
// into a single artifact that we can use to display in the GUI.
type MasterFrontendManager struct {
	config_obj *config_proto.Config

	mu    sync.Mutex
	stats map[string]*FrontendMetrics

	messages []*api_proto.GlobalUserMessage
}

func (self *MasterFrontendManager) SetGlobalMessage(
	message *api_proto.GlobalUserMessage) {
	self.mu.Lock()
	defer self.mu.Unlock()

	found := false
	for idx, item := range self.messages {
		if item.Key == message.Key {
			self.messages[idx] = message
			found = true
		}
	}

	if !found {
		self.messages = append(self.messages, message)
	}
}

func (self *MasterFrontendManager) GetGlobalMessages() []*api_proto.GlobalUserMessage {
	self.mu.Lock()
	defer self.mu.Unlock()

	res := []*api_proto.GlobalUserMessage{}
	for _, item := range self.messages {
		if item.Level != "" {
			res = append(res, item)
		}
	}

	return res
}

func (self *MasterFrontendManager) processMetrics(ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	row_metric, pres := row.Get("Metrics")
	if !pres {
		return nil
	}

	serialized, err := json.Marshal(row_metric)
	if err != nil {
		return err
	}

	metric := &FrontendMetrics{}
	err = json.Unmarshal(serialized, metric)
	if err != nil {
		return err
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	self.stats[metric.NodeName] = metric

	return nil
}

func (self *MasterFrontendManager) GetMinionCount() int {
	res := 0
	self.mu.Lock()
	defer self.mu.Unlock()

	for node_name, metric := range self.stats {
		if node_name != "master" {
			if time.Now().Sub(metric.Timestamp) < 60*time.Second {
				res++
			}
		}
	}
	return res
}

func (self *MasterFrontendManager) prepareOrgStats() (
	map[string]*ordereddict.Dict, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := make(map[string]*ordereddict.Dict)
	now := time.Now()

	// Calculate totals across all frontends
	total_CpuLoadPercent := int64(0)
	total_ProcessResidentMemoryBytes := int64(0)
	active_frontends := int64(0)
	for _, v := range self.stats {
		if now.Sub(v.Timestamp) < 60*time.Second {
			total_CpuLoadPercent += v.CpuLoadPercent
			total_ProcessResidentMemoryBytes += v.ProcessResidentMemoryBytes
			active_frontends++
		}
	}

	// Now build a stats object for each org.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	for _, org := range org_manager.ListOrgs() {
		total := uint64(0)

		for _, v := range self.stats {
			if now.Sub(v.Timestamp) < 60*time.Second {
				count, _ := v.ClientCommsCurrentConnections[org.Id]
				total += count
			}
		}

		result[org.Id] = ordereddict.NewDict().
			Set("TotalFrontends", active_frontends).
			Set("CPUPercent", total_CpuLoadPercent).
			Set("MemoryUse", total_ProcessResidentMemoryBytes).
			Set("client_comms_current_connections", total)
	}

	return result, nil
}

// Every 10 seconds read the cummulative stats and update the
// Server.Monitor.Health artifact.
func (self *MasterFrontendManager) UpdateStats(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(utils.Jitter(10 * time.Second)):
		}

		all_stats, err := self.prepareOrgStats()
		if err != nil {
			logger := logging.GetLogger(self.config_obj,
				&logging.FrontendComponent)
			logger.Error("UpdateStats: %v", err)
			continue
		}

		org_manager, err := services.GetOrgManager()
		if err != nil {
			continue
		}

		for k, v := range all_stats {
			org_config_obj, err := org_manager.GetOrgConfig(k)

			if err != nil {
				continue
			}

			journal, err := services.GetJournal(org_config_obj)
			if err != nil {
				continue
			}

			_ = journal.PushRowsToArtifact(ctx, org_config_obj,
				[]*ordereddict.Dict{v},
				"Server.Monitor.Health/Prometheus", "server", "")
		}
	}
}

// The master does not replicate anywhere.
func (self *MasterFrontendManager) GetMasterAPIClient(ctx context.Context) (
	api_proto.APIClient, func() error, error) {
	return nil, nil, services.FrontendIsMaster
}

func (self *MasterFrontendManager) Start(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("<green>Frontend:</> Server will be master.")

	if config_obj.Datastore == nil {
		return errors.New("Datastore must be specified")
	}

	implementation := config_obj.Datastore.MasterImplementation
	if implementation == "" {
		implementation = config_obj.Datastore.Implementation
	}
	logger.Info("<green>Filestore implementation</> %v.", implementation)
	err := file_store.SetGlobalFilestore(implementation, config_obj)
	if err != nil {
		return err
	}

	err = datastore.SetGlobalDatastore(ctx, implementation, config_obj)
	if err != nil {
		return err
	}

	// Push our metrics to the master node.
	err = PushMetrics(ctx, wg, config_obj, "master")
	if err != nil {
		return err
	}

	go self.UpdateStats(ctx)
	go func() {
		_ = utils.Retry(ctx, func() error {
			return journal.WatchQueueWithCB(ctx, config_obj, wg,
				"Server.Internal.FrontendMetrics",
				"FrontendService",
				self.processMetrics)
		}, 1000, time.Second)
	}()
	return err
}

type MinionFrontendManager struct {
	config_obj *config_proto.Config
	name       string
}

func NewMinionFrontendManager(
	config_obj *config_proto.Config,
	name string) *MinionFrontendManager {
	return &MinionFrontendManager{config_obj: config_obj, name: name}
}

func (self MinionFrontendManager) SetGlobalMessage(
	message *api_proto.GlobalUserMessage) {
}

func (self MinionFrontendManager) GetGlobalMessages() []*api_proto.GlobalUserMessage {
	return nil
}

func (self MinionFrontendManager) GetMinionCount() int {
	return 0
}

func (self MinionFrontendManager) IsMaster() bool {
	return false
}

func (self MinionFrontendManager) GUIBasePath(config_obj *config_proto.Config) string {
	if config_obj.GUI != nil {
		return config_obj.GUI.BasePath
	}
	return ""
}

// The minion frontend replicates to the master node.
func (self MinionFrontendManager) GetMasterAPIClient(ctx context.Context) (
	api_proto.APIClient, func() error, error) {
	// Connect as the SuperUser
	client, closer, err := grpc_client.Factory.GetAPIClient(
		ctx, grpc_client.SuperUser, self.config_obj)
	if err != nil {
		return nil, nil, err
	}

	currentReplicationConnections.Inc()

	return client, func() error {
		defer currentReplicationConnections.Dec()
		return closer()
	}, err
}

func (self *MinionFrontendManager) Start(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	// If no service specification is set, we start only some
	// services on minion frontends.
	if config_obj.Services == nil {
		config_obj.Services = services.MinionServicesSpec()
	}

	self.name = services.GetNodeName(config_obj.Frontend)

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("<green>Frontend:</> Server will be a minion, with ID %v.", self.name)

	implementation := config_obj.Datastore.MinionImplementation
	if implementation == "" {
		implementation = config_obj.Datastore.Implementation
	}

	logger.Info("<green>Filestore implementation</> %v.", implementation)
	err := file_store.SetGlobalFilestore(implementation, config_obj)
	if err != nil {
		return err
	}

	err = datastore.SetGlobalDatastore(ctx, implementation, config_obj)
	if err != nil {
		return err
	}

	// Push our metrics to the master node.
	return PushMetrics(ctx, wg, config_obj, self.name)
}

// Install a frontend manager. This must be the first service created
// in the frontend. The service will determine if we are running in
// master or minion context.
func NewFrontendService(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.FrontendManager, error) {
	if config_obj.Frontend == nil {
		return nil, errors.New("Frontend not configured")
	}

	// Sub orgs just use the same frontend manager
	if !utils.IsRootOrg(config_obj.OrgId) {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return nil, err
		}

		root_org_config, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
		if err != nil {
			return nil, err
		}
		return services.GetFrontendManager(root_org_config)
	}

	if services.IsMaster(config_obj) {
		manager := &MasterFrontendManager{
			config_obj: config_obj,
			stats:      make(map[string]*FrontendMetrics),
		}
		return manager, manager.Start(ctx, wg, config_obj)
	}

	manager := &MinionFrontendManager{config_obj: config_obj}
	return manager, manager.Start(ctx, wg, config_obj)
}

// Selects the node by name from the extra frontends configuration
func SelectFrontend(node string, config_obj *config_proto.Config) error {
	for _, fe := range config_obj.ExtraFrontends {
		fe_name := services.GetNodeName(fe)
		if fe_name == node {
			proto.Merge(config_obj.Frontend, fe)
			return nil
		}
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Error("Frontend %v not found!", node)
	if len(config_obj.ExtraFrontends) > 0 {
		for _, fe := range config_obj.ExtraFrontends {
			fe_name := fmt.Sprintf("%v:%v", fe.Hostname, fe.BindPort)
			logger.Error("Available Frontend %v", fe_name)
		}
	}

	return fmt.Errorf("Frontend %v not found!", node)
}
