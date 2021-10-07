package client_info

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
)

const (
	MAX_CACHE_AGE = 3600
)

type CachedInfo struct {
	record *services.ClientInfo
	age    time.Time
}

func (self CachedInfo) Size() int {
	return 1
}

type ClientInfoManager struct {
	config_obj *config_proto.Config
	lru        *cache.LRUCache
}

func (self *ClientInfoManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Client Info service.")

	// Watch for clients that are deleted and remove from local cache.
	err := journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.ClientDelete", self.ProcessInterrogateResults)

	if err != nil {
		return err
	}

	return journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.Interrogation", self.ProcessInterrogateResults)
}

func (self *ClientInfoManager) ProcessInterrogateResults(
	ctx context.Context, config_obj *config_proto.Config,
	row *ordereddict.Dict) error {
	client_id, pres := row.GetString("ClientId")
	if pres {
		self.lru.Delete(client_id)
	}
	return nil
}

func (self *ClientInfoManager) Flush(client_id string) {
	self.lru.Delete(client_id)
}

func (self *ClientInfoManager) Clear() {
	self.lru.Clear()
}

func (self *ClientInfoManager) Get(client_id string) (*services.ClientInfo, error) {
	cached_any, ok := self.lru.Get(client_id)
	if ok {
		item := cached_any.(*CachedInfo)
		duration := time.Since(item.age)
		if duration.Seconds() < MAX_CACHE_AGE {
			return item.record, nil
		}
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	record := &actions_proto.ClientInfo{}
	client_path_manager := paths.NewClientPathManager(client_id)
	err = db.GetSubject(self.config_obj, client_path_manager.Path(), record)
	if err != nil {
		return nil, err
	}

	os := services.Unknown
	switch record.System {
	case "windows":
		os = services.Windows
	case "linux":
		os = services.Linux
	case "darwin":
		os = services.MacOS
	}

	client_info := &services.ClientInfo{
		Hostname: record.Hostname,
		OS:       os,
		Info:     record,
	}

	self.lru.Set(client_id, &CachedInfo{
		record: client_info,
		age:    time.Now(),
	})
	return client_info, nil
}

func StartClientInfoService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	expected_clients := int64(100)
	if config_obj.Frontend != nil && config_obj.Frontend.Resources != nil {
		expected_clients = config_obj.Frontend.Resources.ExpectedClients
	}

	service := &ClientInfoManager{
		config_obj: config_obj,
		lru:        cache.NewLRUCache(expected_clients),
	}
	services.RegisterClientInfoManager(service)

	return service.Start(ctx, config_obj, wg)
}
