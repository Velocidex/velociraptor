package services

import (
	"context"

	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
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
	Set(ctx context.Context,
		client_info *ClientInfo) error

	Get(ctx context.Context,
		client_id string) (*ClientInfo, error)

	Remove(ctx context.Context, client_id string)

	GetStats(ctx context.Context, client_id string) (*Stats, error)
	UpdateStats(ctx context.Context, client_id string, stats *Stats) error

	// Get the client's tasks and remove them from the queue.
	GetClientTasks(ctx context.Context,
		client_id string) ([]*crypto_proto.VeloMessage, error)

	// Get all the tasks without de-queuing them.
	PeekClientTasks(ctx context.Context,
		client_id string) ([]*crypto_proto.VeloMessage, error)

	QueueMessagesForClient(
		ctx context.Context,
		client_id string,
		req []*crypto_proto.VeloMessage,
		notify bool, /* Also notify the client about the new task */
	) error

	QueueMessageForClient(
		ctx context.Context,
		client_id string,
		req *crypto_proto.VeloMessage,
		notify bool, /* Also notify the client about the new task */
		completion func()) error

	UnQueueMessageForClient(
		ctx context.Context,
		client_id string,
		req *crypto_proto.VeloMessage) error

	// Remove client id from the cache - this is needed when the
	// record chages and we need to force a read from storage.
	Flush(ctx context.Context, client_id string)
}

func GetHostname(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) string {
	client_info_manager, err := GetClientInfoManager(config_obj)
	if err != nil {
		return ""
	}
	info, err := client_info_manager.Get(ctx, client_id)
	if err != nil {
		return ""
	}

	return info.Hostname
}
