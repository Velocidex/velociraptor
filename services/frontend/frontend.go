// This service is responsible for selecting the frontend to use. It
// should be called very early in the frontend start process.

// When we start up, we inspect the file store to find a frontend that
// is not currently running, and then we take over that frontend.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	frontend_proto "www.velocidex.com/golang/velociraptor/services/frontend/proto"
)

type FrontendManager struct {
	mu sync.Mutex

	// All the frontends loaded in the config file.
	frontends map[string]*config_proto.FrontendConfig

	// A constantly refreshing list of active frontends from the
	// data store.
	active_frontends map[string]*frontend_proto.FrontendState

	config_obj *config_proto.Config

	path_manager *FrontendPathManager

	my_state *frontend_proto.FrontendState

	// For now we only allow frontends to be distributed.
	primary_frontend string

	// A list of active frontend URLs
	urls []string

	sample int
}

func GetFrontendName(fe *config_proto.FrontendConfig) string {
	return fmt.Sprintf("%s:%d", fe.Hostname, fe.BindPort)
}

func (self *FrontendManager) addFrontendConfig(fe *config_proto.FrontendConfig) {
	self.mu.Lock()
	defer self.mu.Unlock()

	name := GetFrontendName(fe)
	_, pres := self.frontends[name]
	if pres {
		panic("Config specifies duplicate frontends: " + name)
	}

	self.frontends[name] = fe
}

func (self *FrontendManager) calculateMetrics(now int64) error {
	metrics := &frontend_proto.Metrics{}

	gathering, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
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

				if self.my_state.Metrics != nil {
					delta_cpu := (total_time -
						self.my_state.Metrics.ProcessCpuNanoSecondsTotal)
					delta_t := now - self.my_state.Heartbeat

					if delta_t > 0 && self.my_state.Heartbeat > 0 {
						metrics.CpuLoadPercent = delta_cpu *
							100 / delta_t
					}
				}
				metrics.ProcessCpuNanoSecondsTotal = total_time
			}
		case "client_comms_current_connections":
			if metric.Metric[0].Gauge != nil {
				metrics.ClientCommsCurrentConnections = (int64)(
					*metric.Metric[0].Gauge.Value)
			}

		case "process_resident_memory_bytes":
			if metric.Metric[0].Gauge != nil {
				metrics.ProcessResidentMemoryBytes = (int64)(
					*metric.Metric[0].Gauge.Value)
			}
		}
	}

	self.my_state.Metrics = metrics

	return nil
}

func (self *FrontendManager) setMyState() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	now := time.Now().UnixNano()
	path_manager := self.path_manager.Frontend(self.my_state.Name)

	// Calculate the metrics.
	err = self.calculateMetrics(now)
	if err != nil {
		return err
	}
	self.my_state.Heartbeat = now

	return db.SetSubject(self.config_obj, path_manager, self.my_state)
}

func (self *FrontendManager) clearMyState() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}
	return db.DeleteSubject(self.config_obj,
		self.path_manager.Frontend(self.my_state.Name))
}

func (self *FrontendManager) syncActiveFrontends() error {
	active_frontends := make(map[string]*frontend_proto.FrontendState)

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	now := time.Now().UnixNano()
	children, err := db.ListChildren(self.config_obj,
		self.path_manager.Path(), 0, 1000)
	if err != nil {
		return err
	}

	total_metrics := &frontend_proto.Metrics{}
	urls := make([]string, 0, len(children))
	for _, child := range children {
		state := &frontend_proto.FrontendState{}
		err = db.GetSubject(self.config_obj, child, state)
		if err != nil && err != io.EOF {
			return err
		}

		// Only count frontends that were active at least 30
		// seconds ago.
		if state.Heartbeat < now-30000000000 { // 30 sec
			continue
		}

		active_frontends[state.Name] = state
		urls = append(urls, state.Url)

		total_metrics.CpuLoadPercent += state.Metrics.CpuLoadPercent
		total_metrics.ClientCommsCurrentConnections += state.Metrics.ClientCommsCurrentConnections
		total_metrics.ProcessResidentMemoryBytes += state.Metrics.ProcessResidentMemoryBytes
	}

	// Keep the lock to a minimum.
	self.mu.Lock()
	self.active_frontends = active_frontends
	self.urls = urls
	self.mu.Unlock()

	if self.sample%2 == 0 {
		journal, err := services.GetJournal()
		if err != nil {
			return err
		}

		err = journal.PushRowsToArtifact(self.config_obj,
			[]*ordereddict.Dict{ordereddict.NewDict().
				Set("CPUPercent", total_metrics.CpuLoadPercent).
				Set("MemoryUse", total_metrics.ProcessResidentMemoryBytes).
				Set("client_comms_current_connections",
					total_metrics.ClientCommsCurrentConnections)},
			"Server.Monitor.Health/Prometheus", "server", "")
		if err != nil {
			return err
		}
	}
	self.sample++

	return nil
}

// GetFrontendURL gets a URL of another frontend. If we return this
// frontend's url then we must serve this request ourselves.
func (self *FrontendManager) GetFrontendURL() (string, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if len(self.urls) <= 1 {
		return "", false
	}

	result := self.urls[rand.Intn(len(self.urls))]
	return result, result != self.my_state.Url
}

// Selects a frontend to take on.
func (self *FrontendManager) selectFrontend(node string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)

	heartbeat := time.Now().UnixNano()

	if node != "" {
		conf, pres := self.frontends[node]
		if !pres {
			return errors.New("Unknown node " + node)
		}

		self.my_state = &frontend_proto.FrontendState{
			Name:      node,
			Heartbeat: heartbeat,
			Url:       getURL(conf),
		}
		self.config_obj.Frontend = conf

		logger.Info("Selected frontend configuration %v", node)
		return nil
	}

	for name, conf := range self.frontends {
		active_state, pres := self.active_frontends[name]

		// older than 60 sec or not present - select this frontend.
		if !pres || active_state.Heartbeat < heartbeat-600000000000 {
			self.my_state = &frontend_proto.FrontendState{
				Name:      name,
				Heartbeat: heartbeat,
				Url:       getURL(conf),
			}
			self.config_obj.Frontend = conf

			logger := logging.GetLogger(
				self.config_obj, &logging.FrontendComponent)
			logger.Info("Selected frontend configuration %v", name)

			return nil
		}
	}

	return errors.New("All frontends appear to be active.")
}

func getURL(fe_config *config_proto.FrontendConfig) string {
	if fe_config.BindPort == 443 {
		return fmt.Sprintf("https://%s/", fe_config.Hostname)
	}
	return fmt.Sprintf("https://%s:%d/", fe_config.Hostname,
		fe_config.BindPort)
}

// Install a frontend manager.
func StartFrontendService(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config, node string) error {
	if config_obj.Frontend == nil {
		return errors.New("Frontend not configured")
	}

	var err error

	fe_manager := &FrontendManager{
		frontends:        make(map[string]*config_proto.FrontendConfig),
		active_frontends: make(map[string]*frontend_proto.FrontendState),
		config_obj:       config_obj,
		path_manager:     &FrontendPathManager{},
		primary_frontend: GetFrontendName(config_obj.Frontend),
	}

	services.Frontend = fe_manager

	// If no service specification is set, we start all services
	// on the primary frontend.
	if config_obj.Frontend.ServerServices == nil {
		config_obj.Frontend.ServerServices = &config_proto.ServerServicesConfig{
			HuntManager:       true,
			HuntDispatcher:    true,
			StatsCollector:    true,
			ServerMonitoring:  true,
			ServerArtifacts:   true,
			DynDns:            true,
			Interrogation:     true,
			SanityChecker:     true,
			VfsService:        true,
			UserManager:       true,
			ClientMonitoring:  true,
			MonitoringService: true,
			ApiServer:         true,
			FrontendServer:    true,
			GuiServer:         true,
		}
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	fe_manager.addFrontendConfig(config_obj.Frontend)
	for _, fe_config := range config_obj.ExtraFrontends {
		// Duplicate keys to all frontends.
		fe_config.Certificate = config_obj.Frontend.Certificate
		fe_config.PrivateKey = config_obj.Frontend.PrivateKey
		fe_manager.addFrontendConfig(fe_config)
	}

	// Select a frontend to run as
	for i := 0; i < 10; i++ {
		err = fe_manager.syncActiveFrontends()
		if err != nil {
			return err
		}
		err = fe_manager.selectFrontend(node)
		if err == nil {
			break
		}
		logger.Info("Waiting for frontend slot to become available.")
		select {
		case <-ctx.Done():
			return nil

		case <-time.After(10 * time.Second):
			continue
		}
	}
	if err != nil {
		return err
	}

	// Store the selection in the data store. NOTE this is a small
	// race but frontends do not get restarted that often so maybe
	// it's fine.
	err = fe_manager.setMyState()
	if err != nil {
		return err
	}

	// Secondary frontends do not run services if they are not configured to.
	if config_obj.Frontend.ServerServices == nil {
		config_obj.Frontend.ServerServices = &config_proto.ServerServicesConfig{}
	}

	// Start the update loop
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				// Not a guaranteed removal but if we
				// exit cleanly we can free up the
				// frontend slot.
				err = fe_manager.clearMyState()
				if err != nil {
					logger.Error("Unable to remove frontend: %v", err)
				}
				return

			case <-time.After(10 * time.Second):
				_ = fe_manager.setMyState()
				_ = fe_manager.syncActiveFrontends()
			}
		}
	}()
	notifier := services.GetNotifier()
	if notifier == nil {
		return errors.New("Notifier not ready")
	}

	return services.GetNotifier().NotifyListener(config_obj, "Frontend")
}
