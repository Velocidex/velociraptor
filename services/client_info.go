package services

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

const (
	NOTIFY_CLIENT = true
	ReallyDoIt    = true

	Unknown ClientOS = iota
	Windows
	Linux
	MacOS
)

var (
	DiscardDeleteProgress chan DeleteFlowResponse = nil
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
	*actions_proto.ClientInfo
}

func (self *ClientInfo) Copy() *ClientInfo {
	copy := proto.Clone(self.ClientInfo).(*actions_proto.ClientInfo)
	return &ClientInfo{ClientInfo: copy}
}

func (self *ClientInfo) OS() ClientOS {
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
	ListClients(ctx context.Context) <-chan string

	// Used to set a new client record. To modify an existing record -
	// or set a new one use Modify()
	Set(ctx context.Context,
		client_info *ClientInfo) error

	// Modify a record or set a new one - if the record is not found,
	// modifier will receive a nil client_info. The ClientInfoManager
	// can not be accessed within the modifier function as it is
	// locked for the duration of the change.
	Modify(ctx context.Context, client_id string,
		modifier func(client_info *ClientInfo) (new_record *ClientInfo, err error)) error

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

	// Be able to manipulate the client and server metadata.
	GetMetadata(ctx context.Context,
		client_id string) (*ordereddict.Dict, error)

	SetMetadata(ctx context.Context,
		client_id string, metadata *ordereddict.Dict, principal string) error

	ValidateClientId(client_id string) error

	DeleteClient(ctx context.Context, client_id, principal string,
		progress chan DeleteFlowResponse, really_do_it bool) error
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
