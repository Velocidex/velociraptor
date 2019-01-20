// Manage user's notifications.

package users

import (
	"path"
	"time"

	"github.com/golang/protobuf/jsonpb"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/vfilter"
)

var (
	gUserNotificationManager *UserNotificationManager
)

type UserNotificationManager struct {
	writers              map[string]*csv.CSVWriter
	done                 chan bool
	config_obj           *api_proto.Config
	scope                *vfilter.Scope
	notification_channel chan *api_proto.UserNotification
}

func (self *UserNotificationManager) Start() error {
	go func() {
		for notification := range self.notification_channel {
			self.HandleNotification(notification)
		}
	}()

	return nil
}

func (self *UserNotificationManager) Close() {
	close(self.done)
	close(self.notification_channel)
	self.scope.Close()
	for _, v := range self.writers {
		v.Close()
	}
}

func (self *UserNotificationManager) Notify(message *api_proto.UserNotification) {
	select {
	case <-self.done:
		return

	default:
		self.notification_channel <- message
	}
}

func (self *UserNotificationManager) HandleNotification(
	message *api_proto.UserNotification) {

	writer, pres := self.writers[message.Username]
	if !pres {
		file_store_factory := file_store.GetFileStore(self.config_obj)
		fd, err := file_store_factory.WriteFile(
			path.Join("/users/", message.Username,
				"notifications.csv"))
		if err != nil {
			return
		}

		writer, err = csv.GetCSVWriter(self.scope, fd)
		if err != nil {
			return
		}

		self.writers[message.Username] = writer
	}

	marshaler := &jsonpb.Marshaler{Indent: " "}
	serialized, err := marshaler.MarshalToString(message)
	if err != nil {
		return
	}

	writer.Write(vfilter.NewDict().
		Set("Timestamp", time.Now().UTC().Unix()).
		Set("Message", string(serialized)))
}

func StartUserNotificationManager(config_obj *api_proto.Config) (
	*UserNotificationManager, error) {
	result := &UserNotificationManager{
		config_obj:           config_obj,
		writers:              make(map[string]*csv.CSVWriter),
		done:                 make(chan bool),
		scope:                vfilter.NewScope(),
		notification_channel: make(chan *api_proto.UserNotification),
	}
	err := result.Start()

	if gUserNotificationManager == nil {
		gUserNotificationManager = result
	}

	return result, err
}
