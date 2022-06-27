package services

import (
	"sync"

	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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

// Keep some stats about the client in the cache. These will be synced
// to disk periodically.
type Stats struct {
	Ping                  uint64 `json:"Ping,omitempty"`
	LastHuntTimestamp     uint64 `json:"LastHuntTimestamp,omitempty"`
	LastEventTableVersion uint64 `json:"LastEventTableVersion,omitempty"`
	IpAddress             string `json:"IpAddress,omitempty"`
}

func GetClientInfoManager(config_obj *config_proto.Config) (ClientInfoManager, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).ClientInfoManager()
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
	// Used to set a new client record.
	Set(client_info *ClientInfo) error

	Get(client_id string) (*ClientInfo, error)

	Remove(client_id string)

	GetStats(client_id string) (*Stats, error)
	UpdateStats(client_id string, stats *Stats) error

	// Get the client's tasks and remove them from the queue.
	GetClientTasks(client_id string) ([]*crypto_proto.VeloMessage, error)

	// Get all the tasks without de-queuing them.
	PeekClientTasks(client_id string) ([]*crypto_proto.VeloMessage, error)

	QueueMessagesForClient(
		client_id string,
		req []*crypto_proto.VeloMessage,
		notify bool, /* Also notify the client about the new task */
	) error

	QueueMessageForClient(
		client_id string,
		req *crypto_proto.VeloMessage,
		notify bool, /* Also notify the client about the new task */
		completion func()) error

	UnQueueMessageForClient(
		client_id string,
		req *crypto_proto.VeloMessage) error

	// Remove client id from the cache - this is needed when the
	// record chages and we need to force a read from storage.
	Flush(client_id string)
}

func GetHostname(
	config_obj *config_proto.Config,
	client_id string) string {
	client_info_manager, err := GetClientInfoManager(config_obj)
	if err != nil {
		return ""
	}
	info, err := client_info_manager.Get(client_id)
	if err != nil {
		return ""
	}

	return info.Hostname
}
