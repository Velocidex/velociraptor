package client_info

import (
	"context"
	"sync"
	"time"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
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

	services.RegisterClientInfoManager(&ClientInfoManager{
		config_obj: config_obj,
		lru:        cache.NewLRUCache(expected_clients),
	})

	return nil
}
