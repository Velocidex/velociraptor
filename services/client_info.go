package services

import (
	"errors"
	"sync"

	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

var (
	client_info_manager    ClientInfoManager
	client_info_manager_mu sync.Mutex
)

const (
	Unknown ClientOS = iota
	Windows
	Linux
	MacOS
)

type ClientOS int

func GetClientInfoManager() (ClientInfoManager, error) {
	client_info_manager_mu.Lock()
	defer client_info_manager_mu.Unlock()

	if client_info_manager == nil {
		return nil, errors.New("Client Info Manager not initialized")
	}

	return client_info_manager, nil
}

func RegisterClientInfoManager(m ClientInfoManager) {
	client_info_manager_mu.Lock()
	defer client_info_manager_mu.Unlock()

	client_info_manager = m
}

type ClientInfo struct {
	// The original info from disk
	actions_proto.ClientInfo
}

func (self ClientInfo) Copy() ClientInfo {
	copy := proto.Clone(&self.ClientInfo).(*actions_proto.ClientInfo)
	return ClientInfo{*copy}
}

func (self ClientInfo) OS() ClientOS {
	switch self.System {
	case "windows":
		return Windows
	case "linux":
		return Linux
	case "darwin":
		return MacOS
	}
	return Unknown
}

type ClientInfoManager interface {
	UpdatePing(client_id, ip_address string) error
	Get(client_id string) (*ClientInfo, error)

	// Get the client's tasks and remove them from the queue.
	GetClientTasks(client_id string) ([]*crypto_proto.VeloMessage, error)

	// Get all the tasks without de-queuing them.
	PeekClientTasks(client_id string) ([]*crypto_proto.VeloMessage, error)

	QueueMessageForClient(
		client_id string,
		req *crypto_proto.VeloMessage,
		completion func()) error

	UnQueueMessageForClient(
		client_id string,
		req *crypto_proto.VeloMessage) error

	// Remove client id from the cache - this is needed when the
	// record chages and we need to force a read from storage.
	Flush(client_id string)
}

func GetHostname(client_id string) string {
	client_info_manager, err := GetClientInfoManager()
	if err != nil {
		return ""
	}
	info, err := client_info_manager.Get(client_id)
	if err != nil {
		return ""
	}

	return info.Hostname
}
