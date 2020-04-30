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

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	frontend_proto "www.velocidex.com/golang/velociraptor/frontend/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	fe_manager *FrontendManager
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

func (self *FrontendManager) setMyState() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}
	self.my_state.Heartbeat = time.Now().Unix()
	return db.SetSubject(self.config_obj,
		self.path_manager.Frontend(self.my_state.Name),
		self.my_state)
}

func (self *FrontendManager) syncActiveFrontends() error {
	active_frontends := make(map[string]*frontend_proto.FrontendState)

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	children, err := db.ListChildren(self.config_obj,
		self.path_manager.Path(), 0, 1000)
	for _, child := range children {
		state := &frontend_proto.FrontendState{}
		err = db.GetSubject(self.config_obj, child, state)
		if err != nil {
			return err
		}

		// Only count frontends that were active at least 30
		// seconds ago.
		if state.Heartbeat > now-30 {
			active_frontends[state.Name] = state
		}
	}

	// Keep the lock to a minimum.
	self.mu.Lock()
	self.active_frontends = active_frontends
	self.mu.Unlock()

	return nil
}

// Selects a frontend to take on.
func (self *FrontendManager) selectFrontend(node string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)

	heartbeat := time.Now().Unix()

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

		// older than 10 min or not present - select this frontend.
		if !pres || active_state.Heartbeat < heartbeat-600 {
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

func StartFrontendService(ctx context.Context,
	config_obj *config_proto.Config, node string) error {
	fe_manager = &FrontendManager{
		frontends:        make(map[string]*config_proto.FrontendConfig),
		active_frontends: make(map[string]*frontend_proto.FrontendState),
		config_obj:       config_obj,
		path_manager:     &FrontendPathManager{},
		primary_frontend: GetFrontendName(config_obj.Frontend),
	}

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
		fe_manager.addFrontendConfig(fe_config)
	}

	err := fe_manager.syncActiveFrontends()
	if err != nil {
		return err
	}

	// Select a frontend to run as
	for i := 0; i < 10; i++ {
		err = fe_manager.selectFrontend(node)
		if err == nil {
			break
		}
		logger.Info("Waiting for frontend slot to become available.")
		time.Sleep(10 * time.Second)
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
	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(10 * time.Second):
				fe_manager.setMyState()
				fe_manager.syncActiveFrontends()
			}
		}
	}()

	return services.NotifyClient(config_obj, "Frontend")
}
