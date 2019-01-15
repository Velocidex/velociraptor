// Manage user's notifications.

package users

import (
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
)

type UserNotificationManager struct {
	writers    map[string]*csv.CSVWriter
	wg         sync.WaitGroup
	done       chan bool
	config_obj *api_proto.Config
}

func (self *UserNotificationManager) Start() error {

	return nil
}

func (self *UserNotificationManager) Close() {

}

func StartUserNotificationManager(config_obj *api_proto.Config) (
	*UserNotificationManager, error) {
	result := &UserNotificationManager{
		config_obj: config_obj,
		writers:    make(map[string]*csv.CSVWriter),
		done:       make(chan bool),
		wg:         sync.WaitGroup{},
	}
	err := result.Start()
	return result, err
}
